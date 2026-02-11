package notebook

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/constants"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/components"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	kind = "notebook"

	// ConditionTypeNotebooksCompatible indicates whether notebooks will be impacted by the 3.x upgrade.
	ConditionTypeNotebooksCompatible = "NotebooksCompatible"

	// Image compatibility configuration.
	// Minimum tag version that contains the nginx fix for non-Jupyter notebooks.
	nginxFixMinTag = "2025.2"

	// Minimum RHOAI version for build-based images (RStudio) that contains nginx fix.
	// Used to parse OPENSHIFT_BUILD_REFERENCE values like "rhoai-2.25".
	nginxFixMinRHOAIVersion = "2.25"

	// Label used to identify OOTB notebook images.
	ootbLabel = "app.kubernetes.io/part-of=workbenches"

	// Default namespace where ImageStreams are stored.
	defaultImageStreamNamespace = "redhat-ods-applications"
)

// ImageStatus represents the compatibility status of a notebook's image.
type ImageStatus string

const (
	ImageStatusGood         ImageStatus = "GOOD"
	ImageStatusProblematic  ImageStatus = "PROBLEMATIC"
	ImageStatusCustom       ImageStatus = "CUSTOM"
	ImageStatusVerifyFailed ImageStatus = "VERIFY_FAILED"
)

// NotebookType represents the type of notebook image.
type NotebookType string

const (
	NotebookTypeJupyter    NotebookType = "jupyter"
	NotebookTypeRStudio    NotebookType = "rstudio"
	NotebookTypeCodeServer NotebookType = "codeserver"
	NotebookTypeUnknown    NotebookType = "unknown"
)

// ootbImageStream represents a discovered OOTB ImageStream with its notebook type.
type ootbImageStream struct {
	Name                  string
	Type                  NotebookType
	DockerImageRepository string // .status.dockerImageRepository for path-based matching
}

// notebookAnalysis contains the analysis result for a single notebook.
type notebookAnalysis struct {
	Namespace string
	Name      string
	Status    ImageStatus
	Reason    string
}

// imageAnalysis contains the analysis result for a single container image.
type imageAnalysis struct {
	ContainerName string
	ImageRef      string
	Status        ImageStatus
	Reason        string
}

// imageRef contains parsed components of a container image reference.
type imageRef struct {
	Name     string // Image name (last path component, without tag or digest)
	Tag      string // Tag if present (e.g., "2025.2")
	SHA      string // SHA digest if present (e.g., "sha256:abc...")
	FullPath string // Full path without tag/sha (e.g., "registry/ns/name")
}

// ImpactedWorkloadsCheck identifies Notebook (workbench) instances that will not work in RHOAI 3.x
// due to nginx compatibility requirements in non-Jupyter images.
type ImpactedWorkloadsCheck struct {
	check.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.notebook.impacted-workloads",
			CheckName:        "Workloads :: Notebook :: Impacted Workloads (3.x)",
			CheckDescription: "Identifies Notebook (workbench) instances with images that will not work in RHOAI 3.x",
			CheckRemediation: "Update workbenches with incompatible images to use 2025.2+ versions before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x and Workbenches is Managed.
func (c *ImpactedWorkloadsCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "workbenches", constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.Notebook).
		Run(ctx, func(ctx context.Context, req *validate.WorkloadRequest[*unstructured.Unstructured]) error {
			return c.analyzeNotebooks(ctx, req)
		})
}

// analyzeNotebooks performs image compatibility analysis on all notebooks.
func (c *ImpactedWorkloadsCheck) analyzeNotebooks(
	ctx context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) error {
	notebooks := req.Items

	if len(notebooks) == 0 {
		req.Result.SetCondition(check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("No Notebook (workbench) instances found"),
		))

		return nil
	}

	// Discover OOTB ImageStreams.
	ootbImages, imageStreamData, err := c.discoverOOTBImageStreams(ctx, req.Client)
	if err != nil {
		return fmt.Errorf("discovering OOTB ImageStreams: %w", err)
	}

	// Analyze each notebook.
	var analyses []notebookAnalysis

	for _, nb := range notebooks {
		analysis := c.analyzeNotebook(ctx, req.Client, nb, ootbImages, imageStreamData)
		analyses = append(analyses, analysis)
	}

	// Set conditions based on analysis results.
	c.setConditions(req.Result, analyses)

	// Set impacted objects to only problematic notebooks.
	c.setImpactedObjects(req.Result, analyses)

	return nil
}

// discoverOOTBImageStreams fetches ImageStreams with the OOTB label and determines their notebook types.
func (c *ImpactedWorkloadsCheck) discoverOOTBImageStreams(
	ctx context.Context,
	reader client.Reader,
) (map[string]ootbImageStream, []*unstructured.Unstructured, error) {
	imageStreams, err := reader.List(ctx, resources.ImageStream,
		client.WithNamespace(defaultImageStreamNamespace),
		client.WithLabelSelector(ootbLabel),
	)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return make(map[string]ootbImageStream), nil, nil
		}

		return nil, nil, fmt.Errorf("listing ImageStreams: %w", err)
	}

	ootbImages := make(map[string]ootbImageStream)

	for _, is := range imageStreams {
		name := is.GetName()

		// Skip runtime images.
		if strings.HasPrefix(name, "runtime-") {
			continue
		}

		nbType := c.determineNotebookType(is)
		dockerRepo, _ := jq.Query[string](is, ".status.dockerImageRepository")
		ootbImages[name] = ootbImageStream{
			Name:                  name,
			Type:                  nbType,
			DockerImageRepository: dockerRepo,
		}
	}

	return ootbImages, imageStreams, nil
}

// determineNotebookType determines the notebook type from ImageStream annotations.
// Parses the JSON annotation values for precise matching.
func (c *ImpactedWorkloadsCheck) determineNotebookType(is *unstructured.Unstructured) NotebookType {
	// Check python-dependencies annotation for JupyterLab.
	if c.hasAnnotationWithName(is, "opendatahub.io/notebook-python-dependencies", "jupyterlab") {
		return NotebookTypeJupyter
	}

	// Check for code-server in either annotation (some images use python-dependencies, others use software).
	if c.hasAnnotationWithName(is, "opendatahub.io/notebook-software", "code-server") ||
		c.hasAnnotationWithName(is, "opendatahub.io/notebook-python-dependencies", "code-server") {
		return NotebookTypeCodeServer
	}

	// Check for R/RStudio.
	if c.hasAnnotationWithName(is, "opendatahub.io/notebook-software", "R") {
		return NotebookTypeRStudio
	}

	return NotebookTypeUnknown
}

// hasAnnotationWithName checks if any tag's annotation contains a JSON array element with the given name.
// The annotation value is expected to be a JSON array like: [{"name":"jupyterlab","version":"4.0"}]
// The comparison is case-insensitive to handle variations in naming across ImageStream versions.
// Returns false if the annotation doesn't exist, isn't valid JSON, or doesn't contain the name.
func (c *ImpactedWorkloadsCheck) hasAnnotationWithName(is *unstructured.Unstructured, annotationKey, name string) bool {
	// Query for the annotation value from any tag.
	// Use JQ to: get all tag annotations, parse as JSON, check if any has matching name (case-insensitive).
	query := fmt.Sprintf(
		`.spec.tags[]? | .annotations[%q] // "" | try fromjson | .[]? | select(.name | ascii_downcase == %q) | .name`,
		annotationKey, strings.ToLower(name),
	)

	matchedName, err := jq.Query[string](is, query)
	if err != nil {
		return false
	}

	return strings.EqualFold(matchedName, name)
}

// analyzeNotebook analyzes a single notebook for image compatibility.
// All container images must be compatible for the notebook to be compatible.
func (c *ImpactedWorkloadsCheck) analyzeNotebook(
	ctx context.Context,
	reader client.Reader,
	nb *unstructured.Unstructured,
	ootbImages map[string]ootbImageStream,
	imageStreamData []*unstructured.Unstructured,
) notebookAnalysis {
	ns := nb.GetNamespace()
	name := nb.GetName()

	// Extract all containers from the notebook spec.
	containers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil || len(containers) == 0 {
		return notebookAnalysis{
			Namespace: ns,
			Name:      name,
			Status:    ImageStatusVerifyFailed,
			Reason:    "Could not extract containers from notebook spec",
		}
	}

	// Analyze each container image, skipping known infrastructure sidecars.
	var imageAnalyses []imageAnalysis

	for _, container := range containers {
		containerMap, ok := container.(map[string]any)
		if !ok {
			continue
		}

		containerName, _ := containerMap["name"].(string)
		image, _ := containerMap["image"].(string)

		// Skip known infrastructure/sidecar containers that are not notebook images.
		if isInfrastructureContainer(containerName, image) {
			continue
		}

		if image == "" {
			imageAnalyses = append(imageAnalyses, imageAnalysis{
				ContainerName: containerName,
				Status:        ImageStatusVerifyFailed,
				Reason:        "Container has no image specified",
			})

			continue
		}

		analysis := c.analyzeImage(ctx, reader, image, ootbImages, imageStreamData)
		analysis.ContainerName = containerName
		analysis.ImageRef = image
		imageAnalyses = append(imageAnalyses, analysis)
	}

	// Aggregate results: notebook is PROBLEMATIC if any image is PROBLEMATIC.
	return c.aggregateImageAnalyses(ns, name, imageAnalyses)
}

// analyzeImage analyzes a single container image for compatibility.
// Uses multiple lookup strategies to correlate container images to OOTB ImageStreams:
// 1. dockerImageReference: Exact match against .status.tags[*].items[*].dockerImageReference
// 2. SHA lookup: Match SHA against .status.tags[*].items[*].image
// 3. dockerImageRepository: Match path against .status.dockerImageRepository (internal registry)
// If none match, the image is classified as CUSTOM (user-provided image requiring manual verification).
func (c *ImpactedWorkloadsCheck) analyzeImage(
	ctx context.Context,
	reader client.Reader,
	image string,
	ootbImages map[string]ootbImageStream,
	imageStreamData []*unstructured.Unstructured,
) imageAnalysis {
	// Parse image reference to get name, tag, SHA, and full path.
	ref := parseImageReference(image)

	// Strategy 1: dockerImageReference lookup - exact match against external registry references.
	// Matches container image like: registry.redhat.io/rhoai/...@sha256:xxx
	// Against ImageStream's: .status.tags[*].items[*].dockerImageReference
	lookup := c.findImageStreamByDockerImageRef(image, imageStreamData)
	if lookup.Found {
		ootbIS, isOOTB := ootbImages[lookup.ImageStreamName]
		if isOOTB {
			return c.analyzeOOTBImage(ctx, reader, lookup.ImageStreamName, lookup.Tag, ref.SHA, ootbIS.Type, imageStreamData)
		}
	}

	// Strategy 2: SHA lookup - search all OOTB ImageStreams for this SHA.
	// Matches container image SHA against: .status.tags[*].items[*].image
	if ref.SHA != "" {
		lookup := c.findImageStreamForSHA(ref.SHA, imageStreamData)
		if lookup.Found {
			ootbIS, isOOTB := ootbImages[lookup.ImageStreamName]
			if isOOTB {
				return c.analyzeOOTBImage(ctx, reader, lookup.ImageStreamName, lookup.Tag, ref.SHA, ootbIS.Type, imageStreamData)
			}
		}
	}

	// Strategy 3: dockerImageRepository lookup - match container image path against internal registry path.
	// Matches container image like: image-registry.openshift-image-registry.svc:5000/ns/name:tag
	// Against ImageStream's: .status.dockerImageRepository
	if ootbIS := c.findImageStreamByDockerRepo(ref.FullPath, ootbImages); ootbIS != nil {
		return c.analyzeOOTBImage(ctx, reader, ootbIS.Name, ref.Tag, ref.SHA, ootbIS.Type, imageStreamData)
	}

	// No OOTB correlation found - mark as custom image requiring user verification.
	// We intentionally do NOT use name-based matching as a fallback because an image
	// from any registry could coincidentally have the same name as an OOTB ImageStream.
	return imageAnalysis{
		Status: ImageStatusCustom,
		Reason: fmt.Sprintf("Image '%s' is not a recognized OOTB notebook image", ref.Name),
	}
}

// analyzeOOTBImage analyzes an OOTB notebook image for compatibility.
func (c *ImpactedWorkloadsCheck) analyzeOOTBImage(
	ctx context.Context,
	reader client.Reader,
	imageName, imageTag, imageSHA string,
	nbType NotebookType,
	imageStreamData []*unstructured.Unstructured,
) imageAnalysis {
	// Jupyter images are always compatible.
	if nbType == NotebookTypeJupyter {
		return imageAnalysis{
			Status: ImageStatusGood,
			Reason: "Jupyter-based OOTB image (nginx compatible)",
		}
	}

	// For RStudio, check build reference.
	if nbType == NotebookTypeRStudio {
		return c.analyzeRStudioImageCompat(ctx, reader, imageName, imageTag, imageSHA)
	}

	// For CodeServer and other non-Jupyter images, check tag version.
	return c.analyzeTagBasedImageCompat(imageName, imageTag, imageSHA, nbType, imageStreamData)
}

// imageLookupResult contains the result of looking up an image in ImageStreams.
type imageLookupResult struct {
	ImageStreamName string
	Tag             string
	Found           bool
}

// findImageStreamByDockerImageRef searches all ImageStreams for an exact dockerImageReference match.
// This matches container images against .status.tags[*].items[*].dockerImageReference.
func (c *ImpactedWorkloadsCheck) findImageStreamByDockerImageRef(
	imageRef string,
	imageStreams []*unstructured.Unstructured,
) imageLookupResult {
	if imageRef == "" {
		return imageLookupResult{}
	}

	for _, is := range imageStreams {
		isName := is.GetName()

		statusTags, err := jq.Query[[]any](is, ".status.tags")
		if err != nil {
			continue
		}

		for _, tagData := range statusTags {
			tagMap, ok := tagData.(map[string]any)
			if !ok {
				continue
			}

			tagName, _ := tagMap["tag"].(string)
			items, _ := tagMap["items"].([]any)

			for _, item := range items {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}

				dockerImageRef, _ := itemMap["dockerImageReference"].(string)
				if dockerImageRef == imageRef {
					return imageLookupResult{
						ImageStreamName: isName,
						Tag:             tagName,
						Found:           true,
					}
				}
			}
		}
	}

	return imageLookupResult{}
}

// findImageStreamForSHA searches all ImageStreams for a SHA and returns the ImageStream name and tag.
// This matches against .status.tags[*].items[*].image (the SHA digest).
func (c *ImpactedWorkloadsCheck) findImageStreamForSHA(
	sha string,
	imageStreams []*unstructured.Unstructured,
) imageLookupResult {
	if sha == "" {
		return imageLookupResult{}
	}

	for _, is := range imageStreams {
		isName := is.GetName()

		statusTags, err := jq.Query[[]any](is, ".status.tags")
		if err != nil {
			continue
		}

		for _, tagData := range statusTags {
			tagMap, ok := tagData.(map[string]any)
			if !ok {
				continue
			}

			tagName, _ := tagMap["tag"].(string)
			items, _ := tagMap["items"].([]any)

			for _, item := range items {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}

				itemImage, _ := itemMap["image"].(string)
				// Compare SHA values - both should be in format "sha256:xxx..."
				if itemImage == sha {
					return imageLookupResult{
						ImageStreamName: isName,
						Tag:             tagName,
						Found:           true,
					}
				}
			}
		}
	}

	return imageLookupResult{}
}

// findImageStreamByDockerRepo finds an OOTB ImageStream whose dockerImageRepository matches the container image path.
// This handles images from the internal OpenShift registry where the path matches exactly.
func (c *ImpactedWorkloadsCheck) findImageStreamByDockerRepo(
	imagePath string,
	ootbImages map[string]ootbImageStream,
) *ootbImageStream {
	if imagePath == "" {
		return nil
	}

	for _, is := range ootbImages {
		if is.DockerImageRepository != "" && is.DockerImageRepository == imagePath {
			return &is
		}
	}

	return nil
}

// aggregateImageAnalyses combines individual image analyses into a notebook analysis.
// Returns PROBLEMATIC if any image is PROBLEMATIC, otherwise returns the "best" status.
func (c *ImpactedWorkloadsCheck) aggregateImageAnalyses(
	ns, name string,
	analyses []imageAnalysis,
) notebookAnalysis {
	if len(analyses) == 0 {
		return notebookAnalysis{
			Namespace: ns,
			Name:      name,
			Status:    ImageStatusVerifyFailed,
			Reason:    "No container images found",
		}
	}

	// Check for any PROBLEMATIC images - these block the upgrade.
	var problematicReasons []string

	for _, a := range analyses {
		if a.Status == ImageStatusProblematic {
			if a.ContainerName != "" {
				problematicReasons = append(problematicReasons, fmt.Sprintf("%s: %s", a.ContainerName, a.Reason))
			} else {
				problematicReasons = append(problematicReasons, a.Reason)
			}
		}
	}

	if len(problematicReasons) > 0 {
		return notebookAnalysis{
			Namespace: ns,
			Name:      name,
			Status:    ImageStatusProblematic,
			Reason:    strings.Join(problematicReasons, "; "),
		}
	}

	// Check for VERIFY_FAILED - these need attention but don't block.
	for _, a := range analyses {
		if a.Status == ImageStatusVerifyFailed {
			return notebookAnalysis{
				Namespace: ns,
				Name:      name,
				Status:    ImageStatusVerifyFailed,
				Reason:    a.Reason,
			}
		}
	}

	// Check for CUSTOM - user needs to verify manually.
	for _, a := range analyses {
		if a.Status == ImageStatusCustom {
			return notebookAnalysis{
				Namespace: ns,
				Name:      name,
				Status:    ImageStatusCustom,
				Reason:    a.Reason,
			}
		}
	}

	// All images are GOOD.
	return notebookAnalysis{
		Namespace: ns,
		Name:      name,
		Status:    ImageStatusGood,
		Reason:    "All container images are compatible",
	}
}

// analyzeRStudioImageCompat analyzes an RStudio image by checking its build reference.
func (c *ImpactedWorkloadsCheck) analyzeRStudioImageCompat(
	ctx context.Context,
	reader client.Reader,
	imageName, imageTag, imageSHA string,
) imageAnalysis {
	// Look up the ImageStreamTag to get build reference.
	// Use the tag from the annotation, fall back to "latest" if not available.
	tag := imageTag
	if tag == "" {
		tag = "latest"
	}

	istName := imageName + ":" + tag

	ist, err := reader.GetResource(ctx, resources.ImageStreamTag, istName,
		client.InNamespace(defaultImageStreamNamespace))
	if err != nil {
		return imageAnalysis{
			Status: ImageStatusVerifyFailed,
			Reason: fmt.Sprintf("Could not fetch ImageStreamTag %s: %v", istName, err),
		}
	}

	// Extract OPENSHIFT_BUILD_REFERENCE from the image's environment variables.
	buildRef := c.extractBuildReference(ist)
	if buildRef == "" {
		return imageAnalysis{
			Status: ImageStatusVerifyFailed,
			Reason: fmt.Sprintf("RStudio image %s has no OPENSHIFT_BUILD_REFERENCE", imageName),
		}
	}

	// Check if the current ImageStreamTag points to the same image SHA.
	currentSHA, _ := jq.Query[string](ist, ".image.metadata.name")
	if imageSHA != "" && currentSHA != "" && imageSHA != currentSHA {
		// Notebook is using a different image than current latest.
		return imageAnalysis{
			Status: ImageStatusProblematic,
			Reason: "RStudio image uses stale build (SHA mismatch), rebuild required",
		}
	}

	// Check if build reference is compliant.
	if isCompliantBuildRef(buildRef) {
		return imageAnalysis{
			Status: ImageStatusGood,
			Reason: fmt.Sprintf("RStudio image built from %s (>= rhoai-%s, has nginx fix)", buildRef, nginxFixMinRHOAIVersion),
		}
	}

	return imageAnalysis{
		Status: ImageStatusProblematic,
		Reason: fmt.Sprintf("RStudio image built from %s (< rhoai-%s, lacks nginx fix)", buildRef, nginxFixMinRHOAIVersion),
	}
}

// analyzeTagBasedImageCompat analyzes a non-RStudio image by checking its tag version.
func (c *ImpactedWorkloadsCheck) analyzeTagBasedImageCompat(
	imageName, imageTag, imageSHA string,
	nbType NotebookType,
	imageStreamData []*unstructured.Unstructured,
) imageAnalysis {
	// Use tag from annotation if available, otherwise look up by SHA.
	tag := imageTag
	if tag == "" {
		tag = c.findTagForSHA(imageSHA, imageName, imageStreamData)
	}

	// If we have a valid version tag, check if it's compliant.
	if isValidVersionTag(tag) {
		if isTagGTE(tag, nginxFixMinTag) {
			return imageAnalysis{
				Status: ImageStatusGood,
				Reason: fmt.Sprintf("%s image with tag %s (>= %s, has nginx fix)", nbType, tag, nginxFixMinTag),
			}
		}

		// Tag is below minimum - check if SHA is also tagged with a compliant version.
		compliantTag := c.findCompliantTagForSHA(imageSHA, imageStreamData)
		if compliantTag != "" {
			return imageAnalysis{
				Status: ImageStatusGood,
				Reason: fmt.Sprintf("%s image %s:%s has same SHA as compliant %s", nbType, imageName, tag, compliantTag),
			}
		}

		return imageAnalysis{
			Status: ImageStatusProblematic,
			Reason: fmt.Sprintf("%s image with tag %s (< %s, lacks nginx fix)", nbType, tag, nginxFixMinTag),
		}
	}

	// No valid version tag found - try SHA cross-reference.
	if imageSHA != "" {
		compliantTag := c.findCompliantTagForSHA(imageSHA, imageStreamData)
		if compliantTag != "" {
			return imageAnalysis{
				Status: ImageStatusGood,
				Reason: fmt.Sprintf("%s image has same SHA as compliant %s", nbType, compliantTag),
			}
		}
	}

	return imageAnalysis{
		Status: ImageStatusVerifyFailed,
		Reason: fmt.Sprintf("Could not determine compatibility for %s image %s", nbType, imageName),
	}
}

// extractBuildReference extracts OPENSHIFT_BUILD_REFERENCE from ImageStreamTag.
func (c *ImpactedWorkloadsCheck) extractBuildReference(ist *unstructured.Unstructured) string {
	envVars, err := jq.Query[[]any](ist, ".image.dockerImageMetadata.Config.Env")
	if err != nil {
		return ""
	}

	for _, envVar := range envVars {
		envStr, ok := envVar.(string)
		if !ok {
			continue
		}

		if val, found := strings.CutPrefix(envStr, "OPENSHIFT_BUILD_REFERENCE="); found {
			return val
		}
	}

	return ""
}

// findTagForSHA finds the tag that references the given SHA in the ImageStream.
func (c *ImpactedWorkloadsCheck) findTagForSHA(sha, imageName string, imageStreams []*unstructured.Unstructured) string {
	if sha == "" {
		return ""
	}

	for _, is := range imageStreams {
		if is.GetName() != imageName {
			continue
		}

		statusTags, err := jq.Query[[]any](is, ".status.tags")
		if err != nil {
			continue
		}

		for _, tagData := range statusTags {
			tagMap, ok := tagData.(map[string]any)
			if !ok {
				continue
			}

			tag, _ := tagMap["tag"].(string)
			items, _ := tagMap["items"].([]any)

			for _, item := range items {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}

				itemImage, _ := itemMap["image"].(string)
				if itemImage == sha {
					return tag
				}
			}
		}
	}

	return ""
}

// findCompliantTagForSHA searches all ImageStreams for a compliant tag (>= nginxFixMinTag) that references the given SHA.
func (c *ImpactedWorkloadsCheck) findCompliantTagForSHA(sha string, imageStreams []*unstructured.Unstructured) string {
	if sha == "" {
		return ""
	}

	for _, is := range imageStreams {
		isName := is.GetName()

		statusTags, err := jq.Query[[]any](is, ".status.tags")
		if err != nil {
			continue
		}

		for _, tagData := range statusTags {
			tagMap, ok := tagData.(map[string]any)
			if !ok {
				continue
			}

			tag, _ := tagMap["tag"].(string)

			// Check if this is a compliant version tag.
			if !isValidVersionTag(tag) || !isTagGTE(tag, nginxFixMinTag) {
				continue
			}

			items, _ := tagMap["items"].([]any)

			for _, item := range items {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}

				itemImage, _ := itemMap["image"].(string)
				if itemImage == sha {
					return fmt.Sprintf("%s:%s", isName, tag)
				}
			}
		}
	}

	return ""
}

// setConditions sets the diagnostic condition based on analysis results.
func (c *ImpactedWorkloadsCheck) setConditions(
	dr *result.DiagnosticResult,
	analyses []notebookAnalysis,
) {
	// Count notebooks by status.
	var goodCount, customCount, problematicCount, verifyFailedCount int

	for _, a := range analyses {
		switch a.Status {
		case ImageStatusGood:
			goodCount++
		case ImageStatusCustom:
			customCount++
		case ImageStatusProblematic:
			problematicCount++
		case ImageStatusVerifyFailed:
			verifyFailedCount++
		}
	}

	totalCount := len(analyses)

	// Build multi-line breakdown message.
	message := fmt.Sprintf(`Found %d Notebook(s):
  - %d compatible (OOTB images ready to run in 3.x)
  - %d custom images (user verification needed)
  - %d incompatible (must update before upgrade)
  - %d unverified (could not determine status)`,
		totalCount, goodCount, customCount, problematicCount, verifyFailedCount)

	switch {
	case problematicCount > 0:
		// Notebooks with problematic images block the upgrade.
		dr.SetCondition(check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage("%s", message),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation),
		))

	case customCount > 0 || verifyFailedCount > 0:
		// Some notebooks need user verification but none are blocking.
		dr.SetCondition(check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage("%s", message),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation("Verify custom images are compatible with RHOAI 3.x before upgrading"),
		))

	default:
		// All notebooks are compatible - passing check.
		dr.SetCondition(check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("All %d Notebook(s) use compatible OOTB images", totalCount),
		))
	}
}

// setImpactedObjects sets the ImpactedObjects to only problematic notebooks.
// Uses an empty slice (not nil) to prevent validate.Workloads from auto-populating.
func (c *ImpactedWorkloadsCheck) setImpactedObjects(
	dr *result.DiagnosticResult,
	analyses []notebookAnalysis,
) {
	impacted := make([]metav1.PartialObjectMetadata, 0)

	for _, a := range analyses {
		if a.Status != ImageStatusProblematic {
			continue
		}

		impacted = append(impacted, metav1.PartialObjectMetadata{
			TypeMeta: resources.Notebook.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: a.Namespace,
				Name:      a.Name,
				Annotations: map[string]string{
					"check.opendatahub.io/image-status": string(a.Status),
					"check.opendatahub.io/reason":       a.Reason,
				},
			},
		})
	}

	dr.ImpactedObjects = impacted
}

// parseImageReference parses an image reference and extracts the image name, tag, SHA, and full path.
// Handles formats like:
//   - image-registry.openshift-image-registry.svc:5000/ns/name@sha256:abc...
//   - registry.redhat.io/rhoai/image-name@sha256:abc...
//   - name:tag (from annotation)
func parseImageReference(image string) imageRef {
	var ref imageRef
	pathWithoutDigest := image

	// Extract SHA if present.
	if idx := strings.LastIndex(image, "@"); idx != -1 {
		ref.SHA = image[idx+1:]
		pathWithoutDigest = image[:idx]
	}

	// Extract tag if present (from the path without digest).
	pathForName := pathWithoutDigest
	if idx := strings.LastIndex(pathWithoutDigest, ":"); idx != -1 {
		// Check if this colon is for a tag (not a port in the registry).
		// If there's a "/" after the colon, it's a port; otherwise it's a tag.
		afterColon := pathWithoutDigest[idx+1:]
		if !strings.Contains(afterColon, "/") {
			ref.Tag = afterColon
			pathForName = pathWithoutDigest[:idx]
		}
	}

	// Store full path (without tag/sha) for dockerImageRepository matching.
	ref.FullPath = pathForName

	// Extract just the image name (last path component).
	if idx := strings.LastIndex(pathForName, "/"); idx != -1 {
		ref.Name = pathForName[idx+1:]
	} else {
		ref.Name = pathForName
	}

	return ref
}

// versionTagRegex matches tags in YYYY.N format.
var versionTagRegex = regexp.MustCompile(`^(\d{4})\.(\d+)$`)

// isValidVersionTag checks if a tag is in valid version format (YYYY.N).
func isValidVersionTag(tag string) bool {
	return versionTagRegex.MatchString(tag)
}

// isTagGTE compares two version tags and returns true if tag1 >= tag2.
// Both tags must be in YYYY.N format.
func isTagGTE(tag1, tag2 string) bool {
	matches1 := versionTagRegex.FindStringSubmatch(tag1)
	matches2 := versionTagRegex.FindStringSubmatch(tag2)

	if len(matches1) != 3 || len(matches2) != 3 {
		return false
	}

	year1, _ := strconv.Atoi(matches1[1])
	minor1, _ := strconv.Atoi(matches1[2])
	year2, _ := strconv.Atoi(matches2[1])
	minor2, _ := strconv.Atoi(matches2[2])

	if year1 > year2 {
		return true
	}

	return year1 == year2 && minor1 >= minor2
}

// rhoaiVersionRegex matches RHOAI build references like "rhoai-2.25".
var rhoaiVersionRegex = regexp.MustCompile(`^rhoai-(\d+)\.(\d+)$`)

// isInfrastructureContainer returns true if the container is a known infrastructure sidecar
// that should not be analyzed for notebook image compatibility.
// Both the container name AND image must match known patterns to be skipped.
// This prevents false positives where a user might name their container "oauth-proxy"
// but use a custom image that needs compatibility verification.
func isInfrastructureContainer(containerName, image string) bool {
	// Only skip oauth-proxy sidecars when BOTH conditions are met:
	// 1. Container name is "oauth-proxy"
	// 2. Image contains "ose-oauth-proxy-rhel9" (the official OpenShift oauth-proxy image)
	if containerName == "oauth-proxy" && strings.Contains(image, "ose-oauth-proxy-rhel9") {
		return true
	}

	return false
}

// isCompliantBuildRef checks if a build reference indicates a compliant RHOAI version.
// Parses "rhoai-X.Y" format and compares against nginxFixMinRHOAIVersion.
func isCompliantBuildRef(buildRef string) bool {
	matches := rhoaiVersionRegex.FindStringSubmatch(buildRef)
	if len(matches) != 3 {
		return false
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])

	// Parse minimum version.
	minMatches := strings.Split(nginxFixMinRHOAIVersion, ".")
	if len(minMatches) != 2 {
		return false
	}

	minMajor, _ := strconv.Atoi(minMatches[0])
	minMinor, _ := strconv.Atoi(minMatches[1])

	if major > minMajor {
		return true
	}

	return major == minMajor && minor >= minMinor
}
