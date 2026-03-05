package notebook

import (
	"context"
	"fmt"
	iolib "io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	// Image compatibility configuration.
	// Minimum tag version that contains the nginx fix for non-Jupyter notebooks.
	nginxFixMinTag = "2025.2"

	// Minimum RHOAI version for build-based images (RStudio) that are compatible with 3.x.
	// Used to parse OPENSHIFT_BUILD_REFERENCE values like "rhoai-3.0" or "rhoai-3.0.0".
	nginxFixMinRHOAIVersion = "3.0"

	// Label used to identify OOTB notebook images.
	ootbLabel = "app.kubernetes.io/part-of=workbenches"

	// Annotation that indicates an ImageStream is managed by the RHOAI operator.
	// ImageStreams without this annotation are user-contributed custom images.
	ootbPlatformVersionAnnotation = "platform.opendatahub.io/version"
)

// ImageStatus represents the compatibility status of a notebook's image.
type ImageStatus string

const (
	ImageStatusGood                      ImageStatus = "GOOD"
	ImageStatusPreUpgradeActionRequired  ImageStatus = "PRE_UPGRADE_ACTION_REQUIRED"
	ImageStatusPostUpgradeActionRequired ImageStatus = "POST_UPGRADE_ACTION_REQUIRED"
	ImageStatusCustom                    ImageStatus = "CUSTOM"
	ImageStatusVerifyFailed              ImageStatus = "VERIFY_FAILED"
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
	ImageRef  string // Primary container image reference (for image-centric grouping)
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

// ootbImageInput bundles parameters for OOTB image analysis.
type ootbImageInput struct {
	ImageStreamName string       // Resolved ImageStream name
	Tag             string       // Image tag
	SHA             string       // Image SHA digest
	Type            NotebookType // Notebook type (jupyter, rstudio, codeserver)
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

// FormatVerboseOutput implements check.VerboseOutputFormatter.
// Groups notebook impacted objects by image, then by namespace within each image group.
//
// Output format:
//
//	<status-label>: registry/path:tag (N notebooks)
//	  - namespace: <ns>
//	       - <crd-fqn>/<name>
//	       - <crd-fqn>/<name>
//	  - namespace: <ns>
//	       - <crd-fqn>/<name>
func (c *ImpactedWorkloadsCheck) FormatVerboseOutput(out iolib.Writer, dr *result.DiagnosticResult) {
	crdName := crdFQN(dr)

	// Group notebooks by image reference, preserving insertion order.
	// Within each image group, track notebooks per namespace.
	var groups []imageGroup

	imageIndex := make(map[string]int) // imageRef -> index in groups

	for _, obj := range dr.ImpactedObjects {
		imageRef := obj.Annotations[AnnotationCheckImageRef]
		if imageRef == "" {
			imageRef = "(unknown image)"
		}

		imageStatus := obj.Annotations[AnnotationCheckImageStatus]

		ns := obj.Namespace
		name := obj.Name

		if idx, ok := imageIndex[imageRef]; ok {
			groups[idx].namespaces[ns] = append(groups[idx].namespaces[ns], name)
			groups[idx].count++
		} else {
			imageIndex[imageRef] = len(groups)
			groups = append(groups, imageGroup{
				imageRef:    imageRef,
				imageStatus: imageStatus,
				namespaces:  map[string][]string{ns: {name}},
				count:       1,
			})
		}
	}

	// Sort image groups: problematic (incompatible) before custom, then by imageRef for determinism.
	sort.SliceStable(groups, func(i, j int) bool {
		oi, oj := imageStatusOrder(groups[i].imageStatus), imageStatusOrder(groups[j].imageStatus)
		if oi != oj {
			return oi < oj
		}

		return groups[i].imageRef < groups[j].imageRef
	})

	for _, g := range groups {
		imageLabel := imageStatusLabel(g.imageStatus)
		_, _ = fmt.Fprintf(out, "    %s: %s (%d notebooks)\n", imageLabel, g.imageRef, g.count)

		// Sort namespaces for deterministic output.
		namespaces := make([]string, 0, len(g.namespaces))
		for ns := range g.namespaces {
			namespaces = append(namespaces, ns)
		}
		sort.Strings(namespaces)

		for _, ns := range namespaces {
			names := g.namespaces[ns]
			sort.Strings(names)

			if ns == "" {
				// Cluster-scoped objects listed without namespace header.
				for _, name := range names {
					_, _ = fmt.Fprintf(out, "      - %s/%s\n", crdName, name)
				}
			} else {
				_, _ = fmt.Fprintf(out, "      - namespace: %s\n", ns)
				for _, name := range names {
					_, _ = fmt.Fprintf(out, "           - %s/%s\n", crdName, name)
				}
			}
		}

		_, _ = fmt.Fprintln(out)
	}
}

// imageGroup holds notebooks grouped by their image reference, with sub-grouping by namespace.
type imageGroup struct {
	imageRef    string
	imageStatus string              // CUSTOM, PRE_UPGRADE_ACTION_REQUIRED, etc.
	namespaces  map[string][]string // namespace -> []name
	count       int                 // total notebook count across all namespaces
}

// Image status sort priorities (lower = higher severity).
const (
	imageStatusOrderPreUpgrade = iota
	imageStatusOrderPostUpgrade
	imageStatusOrderCustom
	imageStatusOrderOther
)

// imageStatusOrder returns a sort key for image statuses.
// Lower values sort first: pre-upgrade before post-upgrade before custom.
func imageStatusOrder(status string) int {
	switch ImageStatus(status) {
	case ImageStatusPreUpgradeActionRequired:
		return imageStatusOrderPreUpgrade
	case ImageStatusPostUpgradeActionRequired:
		return imageStatusOrderPostUpgrade
	case ImageStatusCustom:
		return imageStatusOrderCustom
	case ImageStatusGood, ImageStatusVerifyFailed:
		return imageStatusOrderOther
	}

	return imageStatusOrderOther
}

// imageStatusLabel returns a user-friendly label for the image status.
func imageStatusLabel(status string) string {
	switch ImageStatus(status) {
	case ImageStatusGood:
		return "compatible image"
	case ImageStatusCustom:
		return "custom image"
	case ImageStatusPreUpgradeActionRequired:
		return "incompatible image"
	case ImageStatusPostUpgradeActionRequired:
		return "incompatible image (post-upgrade rebuild)"
	case ImageStatusVerifyFailed:
		return "unverified image"
	}

	return "image"
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x; component state is checked via ForComponent in Validate.
func (c *ImpactedWorkloadsCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.Notebook).
		ForComponent(constants.ComponentWorkbenches).
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
	log := newDebugLogger(req.IO, req.Debug)

	log.logf("[notebook] Analyzing %d notebook(s)", len(notebooks))

	if len(notebooks) == 0 {
		req.Result.SetCondition(check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage(MsgNoNotebookInstances),
		))

		return nil
	}

	// Resolve the applications namespace from DSCInitialization.
	appNS, err := client.GetApplicationsNamespace(ctx, req.Client)
	if err != nil {
		return fmt.Errorf("getting applications namespace: %w", err)
	}

	// Discover OOTB ImageStreams.
	ootbImages, imageStreamData, err := c.discoverOOTBImageStreams(ctx, req.Client, appNS, log)
	if err != nil {
		return fmt.Errorf("discovering OOTB ImageStreams: %w", err)
	}

	log.logf("[notebook] Discovered %d OOTB ImageStreams, %d total ImageStreams",
		len(ootbImages), len(imageStreamData))

	// Analyze each notebook.
	var analyses []notebookAnalysis

	for _, nb := range notebooks {
		analysis := c.analyzeNotebook(ctx, req.Client, nb, ootbImages, imageStreamData, appNS, log)
		analyses = append(analyses, analysis)
	}

	// Set conditions based on analysis results.
	c.setConditions(req.Result, analyses, version.MajorMinorLabel(req.TargetVersion))

	// Set impacted objects to only problematic notebooks.
	c.setImpactedObjects(req.Result, analyses)

	return nil
}

// discoverOOTBImageStreams fetches ImageStreams with the OOTB label and determines their notebook types.
func (c *ImpactedWorkloadsCheck) discoverOOTBImageStreams(
	ctx context.Context,
	reader client.Reader,
	appNS string,
	log debugLogger,
) (map[string]ootbImageStream, []*unstructured.Unstructured, error) {
	imageStreams, err := reader.List(ctx, resources.ImageStream,
		client.WithNamespace(appNS),
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

		// Skip ImageStreams without the platform version annotation.
		// These are user-contributed custom images, not operator-managed OOTB images.
		annotations := is.GetAnnotations()
		if annotations == nil || annotations[ootbPlatformVersionAnnotation] == "" {
			log.logf("[notebook]   ImageStream %s: skipped (no %s annotation - custom image)",
				name, ootbPlatformVersionAnnotation)

			continue
		}

		nbType := c.determineNotebookType(is)
		dockerRepo, _ := jq.Query[string](is, ".status.dockerImageRepository")
		ootbImages[name] = ootbImageStream{
			Name:                  name,
			Type:                  nbType,
			DockerImageRepository: dockerRepo,
		}

		log.logf("[notebook]   ImageStream %s: type=%s, dockerRepo=%s", name, nbType, dockerRepo)
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
	appNS string,
	log debugLogger,
) notebookAnalysis {
	ns := nb.GetNamespace()
	name := nb.GetName()

	log.logf("[notebook] Analyzing %s/%s", ns, name)

	// Extract workload containers (infrastructure sidecars already filtered out).
	containers, err := ExtractWorkloadContainers(nb)
	if err != nil || len(containers) == 0 {
		log.logf("[notebook]   %s/%s: VERIFY_FAILED - could not extract containers (err=%v, count=%d)",
			ns, name, err, len(containers))

		return notebookAnalysis{
			Namespace: ns,
			Name:      name,
			Status:    ImageStatusVerifyFailed,
			Reason:    "Could not extract containers from notebook spec",
		}
	}

	// Analyze each container image.
	var imageAnalyses []imageAnalysis

	for _, container := range containers {
		if container.Image == "" {
			log.logf("[notebook]   %s/%s: VERIFY_FAILED - container %s has no image",
				ns, name, container.Name)
			imageAnalyses = append(imageAnalyses, imageAnalysis{
				ContainerName: container.Name,
				Status:        ImageStatusVerifyFailed,
				Reason:        "Container has no image specified",
			})

			continue
		}

		analysis := c.analyzeImage(ctx, reader, container.Image, ootbImages, imageStreamData, appNS, log)
		analysis.ContainerName = container.Name
		analysis.ImageRef = container.Image

		log.logf("[notebook]   %s/%s container %s: status=%s reason=%q",
			ns, name, container.Name, analysis.Status, analysis.Reason)

		imageAnalyses = append(imageAnalyses, analysis)
	}

	// Aggregate results: priority is PRE_UPGRADE > POST_UPGRADE > VERIFY_FAILED > CUSTOM > GOOD.
	return c.aggregateImageAnalyses(ns, name, imageAnalyses)
}

// analyzeImage analyzes a single container image for compatibility.
// Uses multiple lookup strategies to correlate container images to OOTB ImageStreams:
// 1. dockerImageReference: Exact match against .status.tags[*].items[*].dockerImageReference
// 2. SHA lookup: Match SHA against .status.tags[*].items[*].image
// 3. dockerImageRepository: Match path against .status.dockerImageRepository (internal registry)
// 4. spec from.name: Exact match against .spec.tags[*].from.name (disconnected clusters)
// If none match, the image is classified as CUSTOM (user-provided image requiring manual verification).
func (c *ImpactedWorkloadsCheck) analyzeImage(
	ctx context.Context,
	reader client.Reader,
	image string,
	ootbImages map[string]ootbImageStream,
	imageStreamData []*unstructured.Unstructured,
	appNS string,
	log debugLogger,
) imageAnalysis {
	// Parse image reference to get name, tag, SHA, and full path.
	ref := parseImageReference(image)

	log.logf("[notebook]     image=%s parsed: name=%s tag=%s sha=%s fullPath=%s",
		image, ref.Name, ref.Tag, truncateSHA(ref.SHA), ref.FullPath)

	// Strategy 1: dockerImageReference lookup - exact match against external registry references.
	// Matches container image like: registry.redhat.io/rhoai/...@sha256:xxx
	// Against ImageStream's: .status.tags[*].items[*].dockerImageReference
	lookup := c.findImageStreamByDockerImageRef(image, imageStreamData)
	if lookup.Found {
		ootbIS, isOOTB := ootbImages[lookup.ImageStreamName]
		if isOOTB {
			log.logf("[notebook]     Strategy 1 (dockerImageRef) matched: is=%s tag=%s type=%s",
				lookup.ImageStreamName, lookup.Tag, ootbIS.Type)

			return c.analyzeOOTBImage(ctx, reader, ootbImageInput{
				ImageStreamName: lookup.ImageStreamName,
				Tag:             lookup.Tag,
				SHA:             ref.SHA,
				Type:            ootbIS.Type,
			}, imageStreamData, appNS, log)
		}

		log.logf("[notebook]     Strategy 1 matched is=%s but not in OOTB map (possibly runtime image)",
			lookup.ImageStreamName)
	}

	// Strategy 2: SHA lookup - search all OOTB ImageStreams for this SHA.
	// Matches container image SHA against: .status.tags[*].items[*].image
	if ref.SHA == "" {
		log.logf("[notebook]     Strategy 2 skipped: no SHA in image reference")
	} else if lookup := c.findImageStreamForSHA(ref.SHA, imageStreamData); !lookup.Found {
		log.logf("[notebook]     Strategy 2 (SHA lookup): no match for sha=%s", truncateSHA(ref.SHA))
	} else if ootbIS, isOOTB := ootbImages[lookup.ImageStreamName]; isOOTB {
		log.logf("[notebook]     Strategy 2 (SHA lookup) matched: is=%s tag=%s type=%s",
			lookup.ImageStreamName, lookup.Tag, ootbIS.Type)

		return c.analyzeOOTBImage(ctx, reader, ootbImageInput{
			ImageStreamName: lookup.ImageStreamName,
			Tag:             lookup.Tag,
			SHA:             ref.SHA,
			Type:            ootbIS.Type,
		}, imageStreamData, appNS, log)
	} else {
		log.logf("[notebook]     Strategy 2 matched is=%s but not in OOTB map",
			lookup.ImageStreamName)
	}

	// Strategy 3: dockerImageRepository lookup - match container image path against internal registry path.
	// Matches container image like: image-registry.openshift-image-registry.svc:5000/ns/name:tag
	// Against ImageStream's: .status.dockerImageRepository
	if ootbIS, found := c.findImageStreamByDockerRepo(ref.FullPath, ootbImages); found {
		log.logf("[notebook]     Strategy 3 (dockerImageRepo) matched: is=%s tag=%s type=%s",
			ootbIS.Name, ref.Tag, ootbIS.Type)

		return c.analyzeOOTBImage(ctx, reader, ootbImageInput{
			ImageStreamName: ootbIS.Name,
			Tag:             ref.Tag,
			SHA:             ref.SHA,
			Type:            ootbIS.Type,
		}, imageStreamData, appNS, log)
	}

	log.logf("[notebook]     Strategy 3 (dockerImageRepo): no match for path=%s", ref.FullPath)

	// Strategy 4: spec from.name lookup - exact match against source image references.
	// Handles disconnected clusters where .status.tags[*].items is null (import failed)
	// but .spec.tags[*].from.name still contains the operator-configured references.
	lookup = c.findImageStreamBySpecRef(image, imageStreamData)
	if lookup.Found {
		ootbIS, isOOTB := ootbImages[lookup.ImageStreamName]
		if isOOTB {
			log.logf("[notebook]     Strategy 4 (specRef) matched: is=%s tag=%s type=%s",
				lookup.ImageStreamName, lookup.Tag, ootbIS.Type)

			return c.analyzeOOTBImage(ctx, reader, ootbImageInput{
				ImageStreamName: lookup.ImageStreamName,
				Tag:             lookup.Tag,
				SHA:             ref.SHA,
				Type:            ootbIS.Type,
			}, imageStreamData, appNS, log)
		}

		log.logf("[notebook]     Strategy 4 matched is=%s but not in OOTB map", lookup.ImageStreamName)
	}

	log.logf("[notebook]     Strategy 4 (specRef): no match for image=%s", image)

	// No OOTB correlation found - mark as custom image requiring user verification.
	// We intentionally do NOT use name-based matching as a fallback because an image
	// from any registry could coincidentally have the same name as an OOTB ImageStream.
	log.logf("[notebook]     All strategies failed -> CUSTOM")

	return imageAnalysis{
		Status: ImageStatusCustom,
		Reason: fmt.Sprintf("Image '%s' is not a recognized OOTB notebook image", ref.Name),
	}
}

// analyzeOOTBImage analyzes an OOTB notebook image for compatibility.
func (c *ImpactedWorkloadsCheck) analyzeOOTBImage(
	ctx context.Context,
	reader client.Reader,
	input ootbImageInput,
	imageStreamData []*unstructured.Unstructured,
	appNS string,
	log debugLogger,
) imageAnalysis {
	log.logf("[notebook]     analyzeOOTBImage: is=%s tag=%s sha=%s type=%s",
		input.ImageStreamName, input.Tag, truncateSHA(input.SHA), input.Type)

	// Jupyter images are always compatible.
	if input.Type == NotebookTypeJupyter {
		log.logf("[notebook]     -> GOOD (Jupyter always compatible)")

		return imageAnalysis{
			Status: ImageStatusGood,
			Reason: "Jupyter-based OOTB image (nginx compatible)",
		}
	}

	// For RStudio, check build reference.
	if input.Type == NotebookTypeRStudio {
		log.logf("[notebook]     -> checking RStudio build reference")

		return c.analyzeRStudioImageCompat(ctx, reader, input.ImageStreamName, input.Tag, input.SHA, appNS, log)
	}

	// For CodeServer and other non-Jupyter images, check tag version.
	log.logf("[notebook]     -> checking tag-based compatibility (type=%s)", input.Type)

	return c.analyzeTagBasedImageCompat(input.ImageStreamName, input.Tag, input.SHA, input.Type, imageStreamData, log)
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
) (ootbImageStream, bool) {
	if imagePath == "" {
		return ootbImageStream{}, false
	}

	for _, is := range ootbImages {
		if is.DockerImageRepository != "" && is.DockerImageRepository == imagePath {
			return is, true
		}
	}

	return ootbImageStream{}, false
}

// findImageStreamBySpecRef searches all ImageStreams for an exact match of the
// container image against .spec.tags[*].from.name (the source DockerImage reference).
// This handles disconnected clusters where .status.tags[*].items may be null due to
// failed imports, but .spec always contains the operator-configured source references.
func (c *ImpactedWorkloadsCheck) findImageStreamBySpecRef(
	imageRef string,
	imageStreams []*unstructured.Unstructured,
) imageLookupResult {
	if imageRef == "" {
		return imageLookupResult{}
	}

	for _, is := range imageStreams {
		isName := is.GetName()

		specTags, err := jq.Query[[]any](is, ".spec.tags")
		if err != nil {
			continue
		}

		for _, tagData := range specTags {
			tagMap, ok := tagData.(map[string]any)
			if !ok {
				continue
			}

			tagName, _ := tagMap["name"].(string)

			fromMap, ok := tagMap["from"].(map[string]any)
			if !ok {
				continue
			}

			fromKind, _ := fromMap["kind"].(string)
			fromName, _ := fromMap["name"].(string)

			if fromKind == "DockerImage" && fromName == imageRef {
				return imageLookupResult{
					ImageStreamName: isName,
					Tag:             tagName,
					Found:           true,
				}
			}
		}
	}

	return imageLookupResult{}
}

// collectReasonsForStatus collects reasons and the first image ref for analyses matching the given status.
func collectReasonsForStatus(analyses []imageAnalysis, status ImageStatus) ([]string, string) {
	var reasons []string
	var imageRef string

	for _, a := range analyses {
		if a.Status != status {
			continue
		}

		if imageRef == "" {
			imageRef = a.ImageRef
		}

		if a.ContainerName != "" {
			reasons = append(reasons, fmt.Sprintf("%s: %s", a.ContainerName, a.Reason))
		} else {
			reasons = append(reasons, a.Reason)
		}
	}

	return reasons, imageRef
}

// findFirstWithStatus returns the first analysis matching the given status, or nil if none found.
func findFirstWithStatus(analyses []imageAnalysis, status ImageStatus) *imageAnalysis {
	for i := range analyses {
		if analyses[i].Status == status {
			return &analyses[i]
		}
	}

	return nil
}

// aggregateImageAnalyses combines individual image analyses into a notebook analysis.
// Priority: PRE_UPGRADE > POST_UPGRADE > VERIFY_FAILED > CUSTOM > GOOD.
// The ImageRef is set to the image that determines the notebook's status.
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

	// Check for PRE_UPGRADE_ACTION_REQUIRED images - these block the upgrade.
	if reasons, imageRef := collectReasonsForStatus(analyses, ImageStatusPreUpgradeActionRequired); len(reasons) > 0 {
		return notebookAnalysis{
			Namespace: ns,
			Name:      name,
			Status:    ImageStatusPreUpgradeActionRequired,
			Reason:    strings.Join(reasons, "; "),
			ImageRef:  imageRef,
		}
	}

	// Check for POST_UPGRADE_ACTION_REQUIRED images - advisory, fix after upgrade.
	if reasons, imageRef := collectReasonsForStatus(analyses, ImageStatusPostUpgradeActionRequired); len(reasons) > 0 {
		return notebookAnalysis{
			Namespace: ns,
			Name:      name,
			Status:    ImageStatusPostUpgradeActionRequired,
			Reason:    strings.Join(reasons, "; "),
			ImageRef:  imageRef,
		}
	}

	// Check for VERIFY_FAILED - these need attention but don't block.
	if a := findFirstWithStatus(analyses, ImageStatusVerifyFailed); a != nil {
		return notebookAnalysis{
			Namespace: ns,
			Name:      name,
			Status:    ImageStatusVerifyFailed,
			Reason:    a.Reason,
			ImageRef:  a.ImageRef,
		}
	}

	// Check for CUSTOM - user needs to verify manually.
	if a := findFirstWithStatus(analyses, ImageStatusCustom); a != nil {
		return notebookAnalysis{
			Namespace: ns,
			Name:      name,
			Status:    ImageStatusCustom,
			Reason:    a.Reason,
			ImageRef:  a.ImageRef,
		}
	}

	// All images are GOOD - use the first image as the representative.
	return notebookAnalysis{
		Namespace: ns,
		Name:      name,
		Status:    ImageStatusGood,
		Reason:    "All container images are compatible",
		ImageRef:  analyses[0].ImageRef,
	}
}

// analyzeRStudioImageCompat analyzes an RStudio image by checking its build reference.
func (c *ImpactedWorkloadsCheck) analyzeRStudioImageCompat(
	ctx context.Context,
	reader client.Reader,
	imageName, imageTag, imageSHA string,
	appNS string,
	log debugLogger,
) imageAnalysis {
	// Look up the ImageStreamTag to get build reference.
	// Use the tag from the annotation, fall back to "latest" if not available.
	tag := imageTag
	if tag == "" {
		tag = "latest"
	}

	istName := imageName + ":" + tag

	ist, err := reader.GetResource(ctx, resources.ImageStreamTag, istName,
		client.InNamespace(appNS))
	if err != nil {
		log.logf("[notebook]     RStudio: VERIFY_FAILED - could not fetch ImageStreamTag %s: %v", istName, err)

		return imageAnalysis{
			Status: ImageStatusVerifyFailed,
			Reason: fmt.Sprintf("Could not fetch ImageStreamTag %s: %v", istName, err),
		}
	}

	// Extract OPENSHIFT_BUILD_REFERENCE from the image's environment variables.
	buildRef := c.extractBuildReference(ist)
	if buildRef == "" {
		log.logf("[notebook]     RStudio: VERIFY_FAILED - no OPENSHIFT_BUILD_REFERENCE in %s", istName)

		return imageAnalysis{
			Status: ImageStatusVerifyFailed,
			Reason: fmt.Sprintf("RStudio image %s has no OPENSHIFT_BUILD_REFERENCE", imageName),
		}
	}

	log.logf("[notebook]     RStudio: buildRef=%s", buildRef)

	// Check if the current ImageStreamTag points to the same image SHA.
	currentSHA, _ := jq.Query[string](ist, ".image.metadata.name")
	if imageSHA != "" && currentSHA != "" && imageSHA != currentSHA {
		// Notebook is using a different image than current latest.
		return imageAnalysis{
			Status: ImageStatusPostUpgradeActionRequired,
			Reason: "RStudio image uses stale build (SHA mismatch), rebuild required after upgrade",
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
		Status: ImageStatusPostUpgradeActionRequired,
		Reason: fmt.Sprintf("RStudio image built from %s (< rhoai-%s, lacks nginx fix, rebuild after upgrade to 3.x)", buildRef, nginxFixMinRHOAIVersion),
	}
}

// analyzeTagBasedImageCompat analyzes a non-RStudio image by checking its tag version.
func (c *ImpactedWorkloadsCheck) analyzeTagBasedImageCompat(
	imageName, imageTag, imageSHA string,
	nbType NotebookType,
	imageStreamData []*unstructured.Unstructured,
	log debugLogger,
) imageAnalysis {
	// Use tag from annotation if available, otherwise look up by SHA.
	tag := imageTag
	if tag == "" {
		tag = c.findTagForSHA(imageSHA, imageName, imageStreamData)
		log.logf("[notebook]     tag-based: imageTag empty, looked up by SHA -> tag=%q", tag)
	}

	log.logf("[notebook]     tag-based: using tag=%q for %s image %s", tag, nbType, imageName)

	// If we have a valid version tag, check if it's compliant.
	if isValidVersionTag(tag) {
		if isTagGTE(tag, nginxFixMinTag) {
			log.logf("[notebook]     tag-based: tag %s >= %s -> GOOD", tag, nginxFixMinTag)

			return imageAnalysis{
				Status: ImageStatusGood,
				Reason: fmt.Sprintf("%s image with tag %s (>= %s, has nginx fix)", nbType, tag, nginxFixMinTag),
			}
		}

		log.logf("[notebook]     tag-based: tag %s < %s, checking SHA cross-reference", tag, nginxFixMinTag)

		// Tag is below minimum - check if SHA is also tagged with a compliant version.
		compliantTag := c.findCompliantTagForSHA(imageSHA, imageStreamData)
		if compliantTag != "" {
			log.logf("[notebook]     tag-based: SHA cross-ref found compliant tag %s -> GOOD", compliantTag)

			return imageAnalysis{
				Status: ImageStatusGood,
				Reason: fmt.Sprintf("%s image %s:%s has same SHA as compliant %s", nbType, imageName, tag, compliantTag),
			}
		}

		log.logf("[notebook]     tag-based: no compliant SHA cross-ref -> PRE_UPGRADE_ACTION_REQUIRED")

		return imageAnalysis{
			Status: ImageStatusPreUpgradeActionRequired,
			Reason: fmt.Sprintf("%s image with tag %s (< %s, lacks nginx fix)", nbType, tag, nginxFixMinTag),
		}
	}

	log.logf("[notebook]     tag-based: tag %q not valid version format (expected YYYY.N)", tag)

	// No valid version tag found - try SHA cross-reference.
	if imageSHA != "" {
		log.logf("[notebook]     tag-based: trying SHA cross-reference for sha=%s", truncateSHA(imageSHA))

		compliantTag := c.findCompliantTagForSHA(imageSHA, imageStreamData)
		if compliantTag != "" {
			log.logf("[notebook]     tag-based: SHA cross-ref found compliant tag %s -> GOOD", compliantTag)

			return imageAnalysis{
				Status: ImageStatusGood,
				Reason: fmt.Sprintf("%s image has same SHA as compliant %s", nbType, compliantTag),
			}
		}

		log.logf("[notebook]     tag-based: SHA cross-ref found no compliant tag")
	} else {
		log.logf("[notebook]     tag-based: no SHA available for cross-reference")
	}

	log.logf("[notebook]     tag-based: -> VERIFY_FAILED (no valid tag, no SHA cross-ref)")

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

// statusCounter tracks notebook counts and unique images for a single status.
type statusCounter struct {
	count  int
	images map[string]struct{}
}

func newStatusCounter() *statusCounter {
	return &statusCounter{images: make(map[string]struct{})}
}

func (sc *statusCounter) add(imageRef string) {
	sc.count++

	if imageRef != "" {
		sc.images[imageRef] = struct{}{}
	}
}

// countByStatus tallies notebooks and unique images for each status.
func countByStatus(analyses []notebookAnalysis) map[ImageStatus]*statusCounter {
	counters := map[ImageStatus]*statusCounter{
		ImageStatusGood:                      newStatusCounter(),
		ImageStatusCustom:                    newStatusCounter(),
		ImageStatusPreUpgradeActionRequired:  newStatusCounter(),
		ImageStatusPostUpgradeActionRequired: newStatusCounter(),
		ImageStatusVerifyFailed:              newStatusCounter(),
	}

	for _, a := range analyses {
		if sc, ok := counters[a.Status]; ok {
			sc.add(a.ImageRef)
		}
	}

	return counters
}

// setConditions sets the diagnostic condition based on analysis results.
func (c *ImpactedWorkloadsCheck) setConditions(
	dr *result.DiagnosticResult,
	analyses []notebookAnalysis,
	targetVersionLabel string,
) {
	counters := countByStatus(analyses)

	allImages := make(map[string]struct{})
	for _, a := range analyses {
		if a.ImageRef != "" {
			allImages[a.ImageRef] = struct{}{}
		}
	}

	totalCount := len(analyses)
	totalImages := len(allImages)

	// Build multi-line breakdown message with image counts.
	lines := []string{
		fmt.Sprintf(MsgNotebookImageSummary, totalCount, totalImages),
		fmt.Sprintf(MsgCompatibleCount, counters[ImageStatusGood].count, len(counters[ImageStatusGood].images), targetVersionLabel),
		fmt.Sprintf(MsgCustomCount, counters[ImageStatusCustom].count, len(counters[ImageStatusCustom].images)),
		fmt.Sprintf(MsgIncompatibleCount, counters[ImageStatusPreUpgradeActionRequired].count, len(counters[ImageStatusPreUpgradeActionRequired].images)),
		fmt.Sprintf(MsgPostUpgradeCount, counters[ImageStatusPostUpgradeActionRequired].count, len(counters[ImageStatusPostUpgradeActionRequired].images)),
		fmt.Sprintf(MsgUnverifiedCount, counters[ImageStatusVerifyFailed].count, len(counters[ImageStatusVerifyFailed].images)),
	}

	message := strings.Join(lines, "\n")

	switch {
	case counters[ImageStatusPreUpgradeActionRequired].count > 0:
		// Notebooks with pre-upgrade incompatible images — advisory, users may choose to update later.
		dr.SetCondition(check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage("%s", message),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		))

	case counters[ImageStatusPostUpgradeActionRequired].count > 0 ||
		counters[ImageStatusCustom].count > 0 ||
		counters[ImageStatusVerifyFailed].count > 0:
		// Post-upgrade, custom, or unverified notebooks need attention but don't block.
		dr.SetCondition(check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage("%s", message),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(fmt.Sprintf(MsgVerifyCustomImages, targetVersionLabel)),
		))

	default:
		// All notebooks are compatible - passing check.
		dr.SetCondition(check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage(MsgAllNotebooksCompatible, totalCount),
		))
	}
}

// setImpactedObjects sets the ImpactedObjects to incompatible and custom notebooks.
// Custom notebooks are included because they require user verification before upgrade.
// Uses an empty slice (not nil) to prevent validate.Workloads from auto-populating.
func (c *ImpactedWorkloadsCheck) setImpactedObjects(
	dr *result.DiagnosticResult,
	analyses []notebookAnalysis,
) {
	impacted := make([]metav1.PartialObjectMetadata, 0)

	for _, a := range analyses {
		// Include pre-upgrade (must fix), post-upgrade (rebuild after), and custom (needs verification) notebooks.
		if a.Status != ImageStatusPreUpgradeActionRequired &&
			a.Status != ImageStatusPostUpgradeActionRequired &&
			a.Status != ImageStatusCustom {
			continue
		}

		impacted = append(impacted, metav1.PartialObjectMetadata{
			TypeMeta: resources.Notebook.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: a.Namespace,
				Name:      a.Name,
				Annotations: map[string]string{
					AnnotationCheckImageStatus: string(a.Status),
					AnnotationCheckImageRef:    a.ImageRef,
					AnnotationCheckReason:      a.Reason,
				},
			},
		})
	}

	if dr.Annotations == nil {
		dr.Annotations = make(map[string]string)
	}

	dr.Annotations[result.AnnotationResourceCRDName] = resources.Notebook.CRDFQN()
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

// rhoaiVersionRegex matches RHOAI build references like "rhoai-3.0" or "rhoai-2.25.3".
var rhoaiVersionRegex = regexp.MustCompile(`^rhoai-(\d+)\.(\d+)(?:\.\d+)?$`)

// Pre-computed minimum RHOAI version parts from nginxFixMinRHOAIVersion.
// Panics at package load time if the constant has an invalid "X.Y" format.
//
//nolint:gochecknoglobals // Derived from constant at init time; effectively immutable.
var nginxFixMinMajor, nginxFixMinMinor = mustParseVersionParts(nginxFixMinRHOAIVersion)

// isCompliantBuildRef checks if a build reference indicates a compliant RHOAI version.
// Parses "rhoai-X.Y" or "rhoai-X.Y.Z" format and compares against nginxFixMinRHOAIVersion.
func isCompliantBuildRef(buildRef string) bool {
	matches := rhoaiVersionRegex.FindStringSubmatch(buildRef)
	if len(matches) != 3 {
		return false
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])

	if major > nginxFixMinMajor {
		return true
	}

	return major == nginxFixMinMajor && minor >= nginxFixMinMinor
}

// mustParseVersionParts parses a "X.Y" version string into its major and minor
// integer components. Panics if the format is invalid.
//
//nolint:revive // unnamed-result conflicts with nonamedreturns linter
func mustParseVersionParts(v string) (int, int) {
	majorStr, minorStr, ok := strings.Cut(v, ".")
	if !ok {
		panic("invalid version format: " + v)
	}

	major, err := strconv.Atoi(majorStr)
	if err != nil {
		panic("invalid major version in " + v + ": " + err.Error())
	}

	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		panic("invalid minor version in " + v + ": " + err.Error())
	}

	return major, minor
}

// debugLogger provides debug logging when enabled.
// Use debugLogger{} (zero value) for disabled logging.
type debugLogger struct {
	io      iostreams.Interface
	enabled bool
}

// newDebugLogger creates a debugLogger that logs when enabled is true.
func newDebugLogger(io iostreams.Interface, enabled bool) debugLogger {
	return debugLogger{io: io, enabled: enabled}
}

// logf writes a debug message if logging is enabled and io is not nil.
func (d debugLogger) logf(format string, args ...any) {
	if d.enabled && d.io != nil {
		d.io.Errorf(format, args...)
	}
}

// truncateSHA returns a shortened version of a SHA for logging purposes.
// Returns the first 12 characters of the SHA (after "sha256:" prefix if present).
func truncateSHA(sha string) string {
	if sha == "" {
		return ""
	}

	// Remove sha256: prefix if present
	s := strings.TrimPrefix(sha, "sha256:")

	if len(s) > 12 {
		return s[:12] + "..."
	}

	return s
}
