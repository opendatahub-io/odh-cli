package notebook_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():           resources.Notebook.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
	resources.DSCInitialization.GVR():  resources.DSCInitialization.ListKind(),
	resources.ImageStream.GVR():        resources.ImageStream.ListKind(),
	resources.ImageStreamTag.GVR():     resources.ImageStreamTag.ListKind(),
}

// =============================================================================
// Test Constants - Building Blocks
// =============================================================================
// All image references are composed from these building blocks to ensure
// consistency between Notebook images and ImageStream fixtures.

const (
	// Applications namespace (matches the DSCInitialization fixture).
	applicationsNS = "redhat-ods-applications"

	// Registry paths.
	internalRegistry = "image-registry.openshift-image-registry.svc:5000/" + applicationsNS
	externalRegistry = "registry.redhat.io/rhoai"

	// ImageStream names (used in both ImageStream metadata and dockerImageRepository).
	isJupyterDatascience    = "jupyter-datascience"
	isCodeserverDatascience = "codeserver-datascience"
	isRstudioRhel9          = "rstudio-rhel9"

	// SHA digests - the actual image content identifiers.
	shaCompatible          = "sha256:compatible123"
	shaIncompatible        = "sha256:incompatible456"
	shaRstudioCompatible   = "sha256:rstudiocompat"
	shaRstudioIncompatible = "sha256:rstudioincompat"
	shaCustom              = "sha256:customsha123"
	shaUnknown             = "sha256:notinimagestream"

	// Tags - version identifiers.
	tagCurrent  = "2025.2"
	tagPrevious = "2025.1"

	// RStudio build references (determines 3.x compatibility).
	buildRefCompatible   = "rhoai-2.25"
	buildRefIncompatible = "rhoai-2.24"
)

// =============================================================================
// Composed Image References for Notebook Fixtures
// =============================================================================

const (
	// Strategy 3: Internal registry with SHA (dockerImageRepository + SHA).
	jupyterCompatibleSHA      = internalRegistry + "/" + isJupyterDatascience + "@" + shaCompatible
	codeserverCompatibleSHA   = internalRegistry + "/" + isCodeserverDatascience + "@" + shaCompatible
	codeserverIncompatibleSHA = internalRegistry + "/" + isCodeserverDatascience + "@" + shaIncompatible
	rstudioCompatibleSHA      = internalRegistry + "/" + isRstudioRhel9 + "@" + shaRstudioCompatible
	rstudioIncompatibleSHA    = internalRegistry + "/" + isRstudioRhel9 + "@" + shaRstudioIncompatible

	// Strategy 3: Internal registry with tag (dockerImageRepository + tag).
	jupyterCompatibleTag      = internalRegistry + "/" + isJupyterDatascience + ":" + tagCurrent
	codeserverCompatibleTag   = internalRegistry + "/" + isCodeserverDatascience + ":" + tagCurrent
	codeserverIncompatibleTag = internalRegistry + "/" + isCodeserverDatascience + ":" + tagPrevious

	// Strategy 1: External registry (dockerImageReference format).
	jupyterExternalCompatible      = externalRegistry + "/odh-" + isJupyterDatascience + "@" + shaCompatible
	codeserverExternalIncompatible = externalRegistry + "/odh-" + isCodeserverDatascience + "@" + shaIncompatible

	// Custom images - should NOT match any OOTB ImageStream.
	customImageTag = "quay.io/myorg/custom-image:v1.0"
	customImageSHA = "quay.io/myorg/custom-image@" + shaCustom

	// Lookalike images - same name but different registry, should be CUSTOM.
	lookalikeInternal = "my-registry.example.com/" + isJupyterDatascience + ":" + tagCurrent
	lookalikeSHA      = "my-registry.example.com/" + isJupyterDatascience + "@" + shaUnknown

	// User-contributed ImageStream (has workbenches label but no platform.opendatahub.io/version).
	// These should be treated as CUSTOM, not OOTB.
	isUserContributed          = "custom-anythingllm"
	shaUserContributed         = "sha256:usercontrib123"
	userContributedInternalRef = internalRegistry + "/" + isUserContributed + ":1.2.3"

	// Infrastructure sidecar images.
	oauthProxyImage     = "registry.redhat.io/openshift4/ose-oauth-proxy-rhel9@sha256:aa00b068c4c6a2428fd7832d8b53e2c8b0d2bb03799bb2a874ceb00be2bef33f"
	oauthProxyFakeImage = "quay.io/myorg/fake-oauth-proxy:v1.0"
)

// Helper functions to create test fixtures.

func newNotebook(ns, name, image string) *unstructured.Unstructured {
	return newNotebookWithContainers(ns, name, map[string]string{"notebook": image})
}

func newNotebookWithContainers(ns, name string, containers map[string]string) *unstructured.Unstructured {
	containerList := make([]any, 0, len(containers))
	for containerName, image := range containers {
		containerList = append(containerList, map[string]any{
			"name":  containerName,
			"image": image,
		})
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]any{
				"template": map[string]any{
					"spec": map[string]any{
						"containers": containerList,
					},
				},
			},
		},
	}
}

func newImageStream(name string, nbType string) *unstructured.Unstructured {
	// Build annotations based on notebook type.
	var pythonDeps, software string

	switch nbType {
	case "jupyter":
		pythonDeps = `[{"name":"jupyterlab","version":"4.0"}]`
	case "codeserver":
		software = `[{"name":"code-server","version":"4.0"}]`
	case "rstudio":
		software = `[{"name":"R","version":"4.0"}]`
	}

	// Build dockerImageRepository for internal registry matching (Strategy 3).
	dockerImageRepo := internalRegistry + "/" + name

	// Build external registry base for dockerImageReference matching (Strategy 1).
	// This simulates how real ImageStreams reference external images.
	externalImageBase := externalRegistry + "/odh-" + name

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ImageStream.APIVersion(),
			"kind":       resources.ImageStream.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": "redhat-ods-applications",
				"labels": map[string]any{
					"app.kubernetes.io/part-of": "workbenches",
				},
				"annotations": map[string]any{
					// OOTB images are managed by the operator and have this annotation.
					"platform.opendatahub.io/version": "2.25.1",
				},
			},
			"spec": map[string]any{
				"tags": []any{
					map[string]any{
						"name": tagCurrent,
						"annotations": map[string]any{
							"opendatahub.io/notebook-python-dependencies": pythonDeps,
							"opendatahub.io/notebook-software":            software,
						},
					},
				},
			},
			"status": map[string]any{
				"dockerImageRepository": dockerImageRepo,
				"tags": []any{
					map[string]any{
						"tag": tagCurrent,
						"items": []any{
							map[string]any{
								"image":                shaCompatible,
								"dockerImageReference": externalImageBase + "@" + shaCompatible,
							},
						},
					},
					map[string]any{
						"tag": tagPrevious,
						"items": []any{
							map[string]any{
								"image":                shaIncompatible,
								"dockerImageReference": externalImageBase + "@" + shaIncompatible,
							},
						},
					},
				},
			},
		},
	}
}

// newUserContributedImageStream creates an ImageStream that has the workbenches label
// but does NOT have the platform.opendatahub.io/version annotation.
// This simulates user-contributed custom images that should be treated as CUSTOM.
func newUserContributedImageStream(name string) *unstructured.Unstructured {
	dockerImageRepo := internalRegistry + "/" + name

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ImageStream.APIVersion(),
			"kind":       resources.ImageStream.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": "redhat-ods-applications",
				"labels": map[string]any{
					// Has the workbenches label (would normally be included in OOTB discovery)
					"app.kubernetes.io/part-of": "workbenches",
				},
				// NOTE: No "annotations" with platform.opendatahub.io/version
				// This is the key difference from OOTB ImageStreams
			},
			"status": map[string]any{
				"dockerImageRepository": dockerImageRepo,
				"tags": []any{
					map[string]any{
						"tag": "1.2.3",
						"items": []any{
							map[string]any{
								"image":                shaUserContributed,
								"dockerImageReference": "quay.io/user/" + name + "@" + shaUserContributed,
							},
						},
					},
				},
			},
		},
	}
}

func newRStudioImageStreamTag(imageName, buildRef, sha string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ImageStreamTag.APIVersion(),
			"kind":       resources.ImageStreamTag.Kind,
			"metadata": map[string]any{
				"name":      imageName + ":latest",
				"namespace": "redhat-ods-applications",
			},
			"image": map[string]any{
				"metadata": map[string]any{
					"name": sha,
				},
				"dockerImageMetadata": map[string]any{
					"Config": map[string]any{
						"Env": []any{
							"OPENSHIFT_BUILD_REFERENCE=" + buildRef,
							"OPENSHIFT_BUILD_COMMIT=abc123",
						},
					},
				},
			},
		},
	}
}

func TestImpactedWorkloadsCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeNotebooksCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No Notebook (workbench) instances found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestImpactedWorkloadsCheck_SingleNotebook(t *testing.T) {
	tests := []struct {
		name           string
		objects        func() []*unstructured.Unstructured
		expectedStatus metav1.ConditionStatus
		expectedReason string
		expectedImpact resultpkg.Impact
		expectImpacted bool
	}{
		{
			name: "Jupyter_Compliant",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
					newNotebook("test-ns", "jupyter-nb", jupyterCompatibleSHA),
				}
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: check.ReasonVersionCompatible,
			expectedImpact: resultpkg.ImpactNone,
			expectImpacted: false,
		},
		{
			name: "RStudio_CompliantBuildRef",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isRstudioRhel9, "rstudio"),
					newRStudioImageStreamTag(isRstudioRhel9, buildRefCompatible, shaRstudioCompatible),
					newNotebook("test-ns", "rstudio-nb", rstudioCompatibleSHA),
				}
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: check.ReasonVersionCompatible,
			expectedImpact: resultpkg.ImpactNone,
			expectImpacted: false,
		},
		{
			name: "RStudio_NonCompliantBuildRef",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isRstudioRhel9, "rstudio"),
					newRStudioImageStreamTag(isRstudioRhel9, buildRefIncompatible, shaRstudioIncompatible),
					newNotebook("test-ns", "rstudio-nb", rstudioIncompatibleSHA),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactBlocking,
			expectImpacted: true,
		},
		{
			name: "CodeServer_CompliantTag",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isCodeserverDatascience, "codeserver"),
					newNotebook("test-ns", "codeserver-nb", codeserverCompatibleSHA),
				}
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: check.ReasonVersionCompatible,
			expectedImpact: resultpkg.ImpactNone,
			expectImpacted: false,
		},
		{
			name: "CodeServer_NonCompliantTag",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isCodeserverDatascience, "codeserver"),
					newNotebook("test-ns", "codeserver-nb", codeserverIncompatibleSHA),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactBlocking,
			expectImpacted: true,
		},
		{
			name: "CustomImage",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newNotebook("test-ns", "custom-nb", customImageTag),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactAdvisory,
			expectImpacted: true, // Custom images are included in impacted objects for user verification
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			objects := append(tc.objects(), testutil.NewDSCI(applicationsNS))

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        objects,
				CurrentVersion: "2.17.0",
				TargetVersion:  "3.0.0",
			})

			impactedCheck := notebook.NewImpactedWorkloadsCheck()
			result, err := impactedCheck.Validate(ctx, target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.Status.Conditions).To(HaveLen(1))
			g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
				"Type":   Equal(notebook.ConditionTypeNotebooksCompatible),
				"Status": Equal(tc.expectedStatus),
				"Reason": Equal(tc.expectedReason),
			}))
			g.Expect(result.Status.Conditions[0].Impact).To(Equal(tc.expectedImpact))

			if tc.expectImpacted {
				g.Expect(result.ImpactedObjects).To(HaveLen(1))
			} else {
				g.Expect(result.ImpactedObjects).To(BeEmpty())
			}
		})
	}
}

func TestImpactedWorkloadsCheck_MultiContainer(t *testing.T) {
	tests := []struct {
		name           string
		objects        func() []*unstructured.Unstructured
		containers     map[string]string
		expectedStatus metav1.ConditionStatus
		expectedImpact resultpkg.Impact
	}{
		{
			name: "AllGood",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			containers: map[string]string{
				"notebook": jupyterCompatibleSHA,
				"sidecar":  jupyterCompatibleSHA,
			},
			expectedStatus: metav1.ConditionTrue,
			expectedImpact: resultpkg.ImpactNone,
		},
		{
			name: "OneProblematic",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
					newImageStream(isCodeserverDatascience, "codeserver"),
				}
			},
			containers: map[string]string{
				"notebook": jupyterCompatibleSHA,
				"sidecar":  codeserverIncompatibleSHA,
			},
			expectedStatus: metav1.ConditionFalse,
			expectedImpact: resultpkg.ImpactBlocking,
		},
		{
			name: "OneCustom",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			containers: map[string]string{
				"notebook": jupyterCompatibleSHA,
				"sidecar":  customImageTag,
			},
			expectedStatus: metav1.ConditionFalse,
			expectedImpact: resultpkg.ImpactAdvisory,
		},
		{
			name: "CustomAndProblematic",
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isCodeserverDatascience, "codeserver"),
				}
			},
			containers: map[string]string{
				"notebook": customImageTag,
				"sidecar":  codeserverIncompatibleSHA,
			},
			expectedStatus: metav1.ConditionFalse,
			expectedImpact: resultpkg.ImpactBlocking,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			objects := tc.objects()
			objects = append(objects,
				newNotebookWithContainers("test-ns", "multi-nb", tc.containers),
				testutil.NewDSCI(applicationsNS),
			)

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        objects,
				CurrentVersion: "2.17.0",
				TargetVersion:  "3.0.0",
			})

			impactedCheck := notebook.NewImpactedWorkloadsCheck()
			result, err := impactedCheck.Validate(ctx, target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.Status.Conditions).To(HaveLen(1))
			g.Expect(result.Status.Conditions[0].Condition.Status).To(Equal(tc.expectedStatus))
			g.Expect(result.Status.Conditions[0].Impact).To(Equal(tc.expectedImpact))

			// Blocking or Advisory impact means notebook is in ImpactedObjects.
			// Advisory includes custom images that need user verification.
			if tc.expectedImpact == resultpkg.ImpactBlocking || tc.expectedImpact == resultpkg.ImpactAdvisory {
				g.Expect(result.ImpactedObjects).To(HaveLen(1))
				g.Expect(result.ImpactedObjects[0].Name).To(Equal("multi-nb"))
			} else {
				g.Expect(result.ImpactedObjects).To(BeEmpty())
			}
		})
	}
}

func TestImpactedWorkloadsCheck_MixedNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	jupyterIS := newImageStream(isJupyterDatascience, "jupyter")
	rstudioIS := newImageStream(isRstudioRhel9, "rstudio")
	rstudioISTBad := newRStudioImageStreamTag(isRstudioRhel9, buildRefIncompatible, shaRstudioIncompatible)

	jupyterNb := newNotebook("ns1", "jupyter-nb", jupyterCompatibleSHA)
	rstudioNb := newNotebook("ns2", "rstudio-nb", rstudioIncompatibleSHA)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects: []*unstructured.Unstructured{
			testutil.NewDSCI(applicationsNS),
			jupyterIS, rstudioIS, rstudioISTBad, jupyterNb, rstudioNb,
		},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeNotebooksCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": ContainSubstring("2 Notebook(s)"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))

	// Only RStudio notebook is impacted.
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("rstudio-nb"))
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	impactedCheck := notebook.NewImpactedWorkloadsCheck()

	g.Expect(impactedCheck.ID()).To(Equal("workloads.notebook.impacted-workloads"))
	g.Expect(impactedCheck.Name()).To(Equal("Workloads :: Notebook :: Impacted Workloads (3.x)"))
	g.Expect(impactedCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(impactedCheck.Description()).ToNot(BeEmpty())
}

func TestImpactedWorkloadsCheck_CanApply(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		targetVersion  string
		workbenches    string
		expected       bool
	}{
		{
			name:           "LintMode_SameVersion",
			currentVersion: "2.17.0",
			targetVersion:  "2.17.0",
			workbenches:    "Managed",
			expected:       false,
		},
		{
			name:           "Upgrade2xTo3x_Managed",
			currentVersion: "2.17.0",
			targetVersion:  "3.0.0",
			workbenches:    "Managed",
			expected:       true,
		},
		{
			name:           "Upgrade2xTo3x_Removed",
			currentVersion: "2.17.0",
			targetVersion:  "3.0.0",
			workbenches:    "Removed",
			expected:       false,
		},
		{
			name:           "Upgrade3xTo3x",
			currentVersion: "3.0.0",
			targetVersion:  "3.1.0",
			workbenches:    "Managed",
			expected:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": tc.workbenches})},
				CurrentVersion: tc.currentVersion,
				TargetVersion:  tc.targetVersion,
			})

			chk := notebook.NewImpactedWorkloadsCheck()
			canApply, err := chk.CanApply(t.Context(), target)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.expected))
		})
	}
}

func TestImpactedWorkloadsCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}

// TestImpactedWorkloadsCheck_LookupStrategies tests all three image lookup strategies:
// 1. dockerImageReference - exact match against .status.tags[*].items[*].dockerImageReference
// 2. SHA lookup - match SHA against .status.tags[*].items[*].image
// 3. dockerImageRepository - match path against .status.dockerImageRepository.
func TestImpactedWorkloadsCheck_LookupStrategies(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		image          string
		objects        func() []*unstructured.Unstructured
		expectedStatus metav1.ConditionStatus
		expectedReason string
		expectedImpact resultpkg.Impact
	}{
		// Strategy 1: dockerImageReference - exact match against external registry
		{
			name:        "Strategy1_ExternalRegistry_Compliant",
			description: "External registry image matching dockerImageReference (Jupyter)",
			image:       jupyterExternalCompatible,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: check.ReasonVersionCompatible,
			expectedImpact: resultpkg.ImpactNone,
		},
		{
			name:        "Strategy1_ExternalRegistry_NonCompliant",
			description: "External registry image matching dockerImageReference (CodeServer old)",
			image:       codeserverExternalIncompatible,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isCodeserverDatascience, "codeserver"),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactBlocking,
		},

		// Strategy 2: SHA lookup - match by SHA digest
		{
			name:        "Strategy2_SHA_Compliant",
			description: "Internal registry with SHA matching .status.tags[*].items[*].image",
			image:       jupyterCompatibleSHA,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: check.ReasonVersionCompatible,
			expectedImpact: resultpkg.ImpactNone,
		},
		{
			name:        "Strategy2_SHA_NonCompliant",
			description: "Internal registry with SHA matching old version",
			image:       codeserverIncompatibleSHA,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isCodeserverDatascience, "codeserver"),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactBlocking,
		},

		// Strategy 3: dockerImageRepository with tag
		{
			name:        "Strategy3_Tag_Compliant",
			description: "Internal registry with tag matching dockerImageRepository",
			image:       jupyterCompatibleTag,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: check.ReasonVersionCompatible,
			expectedImpact: resultpkg.ImpactNone,
		},
		{
			name:        "Strategy3_Tag_NonCompliant",
			description: "Internal registry with old tag",
			image:       codeserverIncompatibleTag,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isCodeserverDatascience, "codeserver"),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactBlocking,
		},

		// Non-OOTB images - should be classified as CUSTOM
		{
			name:        "Custom_WithTag",
			description: "Custom image with tag format - not in any ImageStream",
			image:       customImageTag,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactAdvisory,
		},
		{
			name:        "Custom_WithSHA",
			description: "Custom image with SHA format - SHA not in any ImageStream",
			image:       customImageSHA,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactAdvisory,
		},

		// Lookalike images - same name but different registry, should NOT match
		{
			name:        "Lookalike_SameName_DifferentRegistry",
			description: "Image with same name as OOTB but from different registry - should be CUSTOM",
			image:       lookalikeInternal,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactAdvisory,
		},
		{
			name:        "Lookalike_SameName_UnknownSHA",
			description: "Image with same name as OOTB but SHA not in ImageStream - should be CUSTOM",
			image:       lookalikeSHA,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					newImageStream(isJupyterDatascience, "jupyter"),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactAdvisory,
		},

		// User-contributed ImageStreams - have workbenches label but no platform.opendatahub.io/version
		// These should be excluded from OOTB discovery and treated as CUSTOM
		{
			name:        "UserContributed_NoPlatformVersion",
			description: "ImageStream has workbenches label but no platform.opendatahub.io/version - should be CUSTOM",
			image:       userContributedInternalRef,
			objects: func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{
					// Include both a real OOTB ImageStream and a user-contributed one
					newImageStream(isJupyterDatascience, "jupyter"),
					newUserContributedImageStream(isUserContributed),
				}
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: check.ReasonWorkloadsImpacted,
			expectedImpact: resultpkg.ImpactAdvisory, // CUSTOM = Advisory, not Blocking
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			objects := tc.objects()
			objects = append(objects,
				newNotebook("test-ns", "test-nb", tc.image),
				testutil.NewDSCI(applicationsNS),
			)

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        objects,
				CurrentVersion: "2.17.0",
				TargetVersion:  "3.0.0",
			})

			impactedCheck := notebook.NewImpactedWorkloadsCheck()
			result, err := impactedCheck.Validate(ctx, target)

			g.Expect(err).ToNot(HaveOccurred(), "Test: %s - %s", tc.name, tc.description)
			g.Expect(result.Status.Conditions).To(HaveLen(1))
			g.Expect(result.Status.Conditions[0].Condition.Status).To(Equal(tc.expectedStatus),
				"Test: %s - expected status %v", tc.name, tc.expectedStatus)
			g.Expect(result.Status.Conditions[0].Condition.Reason).To(Equal(tc.expectedReason),
				"Test: %s - expected reason %s", tc.name, tc.expectedReason)
			g.Expect(result.Status.Conditions[0].Impact).To(Equal(tc.expectedImpact),
				"Test: %s - expected impact %v", tc.name, tc.expectedImpact)
		})
	}
}

// TestImpactedWorkloadsCheck_InfrastructureContainerFiltering tests that oauth-proxy sidecars
// are correctly filtered when BOTH container name AND image match, but NOT when only one matches.
func TestImpactedWorkloadsCheck_InfrastructureContainerFiltering(t *testing.T) {
	tests := []struct {
		name           string
		description    string
		containers     map[string]string
		expectedStatus metav1.ConditionStatus
		expectedImpact resultpkg.Impact
	}{
		{
			name:        "OAuthProxy_CorrectNameAndImage_Skipped",
			description: "oauth-proxy container with ose-oauth-proxy-rhel9 image should be skipped",
			containers: map[string]string{
				"notebook":    jupyterCompatibleSHA,
				"oauth-proxy": oauthProxyImage,
			},
			expectedStatus: metav1.ConditionTrue,
			expectedImpact: resultpkg.ImpactNone,
		},
		{
			name:        "OAuthProxy_CorrectNameWrongImage_NotSkipped",
			description: "oauth-proxy container with non-standard image should NOT be skipped (CUSTOM)",
			containers: map[string]string{
				"notebook":    jupyterCompatibleSHA,
				"oauth-proxy": oauthProxyFakeImage,
			},
			expectedStatus: metav1.ConditionFalse,
			expectedImpact: resultpkg.ImpactAdvisory, // CUSTOM = Advisory
		},
		{
			name:        "OAuthProxy_WrongNameCorrectImage_NotSkipped",
			description: "Different container name with oauth-proxy image should NOT be skipped (CUSTOM)",
			containers: map[string]string{
				"notebook":      jupyterCompatibleSHA,
				"my-auth-proxy": oauthProxyImage,
			},
			expectedStatus: metav1.ConditionFalse,
			expectedImpact: resultpkg.ImpactAdvisory, // CUSTOM = Advisory
		},
		{
			name:        "OAuthProxy_WrongNameWrongImage_NotSkipped",
			description: "Different container name with different image should NOT be skipped (CUSTOM)",
			containers: map[string]string{
				"notebook": jupyterCompatibleSHA,
				"sidecar":  customImageTag,
			},
			expectedStatus: metav1.ConditionFalse,
			expectedImpact: resultpkg.ImpactAdvisory, // CUSTOM = Advisory
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			objects := []*unstructured.Unstructured{
				testutil.NewDSCI(applicationsNS),
				newImageStream(isJupyterDatascience, "jupyter"),
				newNotebookWithContainers("test-ns", "test-nb", tc.containers),
			}

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        objects,
				CurrentVersion: "2.17.0",
				TargetVersion:  "3.0.0",
			})

			impactedCheck := notebook.NewImpactedWorkloadsCheck()
			result, err := impactedCheck.Validate(ctx, target)

			g.Expect(err).ToNot(HaveOccurred(), "Test: %s - %s", tc.name, tc.description)
			g.Expect(result.Status.Conditions).To(HaveLen(1))
			g.Expect(result.Status.Conditions[0].Condition.Status).To(Equal(tc.expectedStatus),
				"Test: %s - expected status %v", tc.name, tc.expectedStatus)
			g.Expect(result.Status.Conditions[0].Impact).To(Equal(tc.expectedImpact),
				"Test: %s - expected impact %v", tc.name, tc.expectedImpact)
		})
	}
}
