package notebook

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// Container state values stored in AnnotationCheckContainerState.
const (
	ContainerStateRunning = "running"
	ContainerStateWaiting = "waiting"
)

// NonStoppedWorkloadsCheck detects Notebook CRs that are not in a stopped state.
// A Notebook is considered stopped when it has the kubeflow-resource-stopped annotation.
// Non-stopped notebooks are classified as "running" or "waiting" based on
// their .status.containerState field.
type NonStoppedWorkloadsCheck struct {
	check.BaseCheck

	// namespaceRequesters maps namespace names to their openshift.io/requester annotation value.
	// Populated by the output renderer via SetNamespaceRequesters before FormatVerboseOutput is called.
	namespaceRequesters map[string]string
}

func NewNonStoppedWorkloadsCheck() *NonStoppedWorkloadsCheck {
	return &NonStoppedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeWorkloadState,
			CheckID:          "workloads.notebook.non-stopped-workloads",
			CheckName:        "Workloads :: Notebook :: Non-Stopped Workloads",
			CheckDescription: "Detects Notebook CRs that are not stopped on the cluster",
			CheckRemediation: "Save all pending work in running Notebooks, then stop them before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x.
func (c *NonStoppedWorkloadsCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate lists all Notebooks and reports an advisory for any that are not stopped.
func (c *NonStoppedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.Notebook).
		ForComponent(constants.ComponentWorkbenches).
		Filter(isNotStopped).
		Run(ctx, c.analyzeNonStoppedWorkloads)
}

// isNotStopped returns true when the Notebook does not have the kubeflow-resource-stopped annotation.
func isNotStopped(nb *unstructured.Unstructured) (bool, error) {
	annotations := nb.GetAnnotations()
	if annotations == nil {
		return true, nil
	}

	_, stopped := annotations[AnnotationKubeflowResourceStopped]

	return !stopped, nil
}

// notebookState holds the classified state of a non-stopped notebook.
type notebookState struct {
	state      string // "running" or "waiting"
	waitReason string // only set when state is "waiting"
}

// classifyNotebook determines the state of a non-stopped notebook
// from its .status.containerState field.
func classifyNotebook(nb *unstructured.Unstructured) notebookState {
	// Check for .status.containerState.running
	if running, _ := jq.Query[map[string]any](nb, ".status.containerState.running"); running != nil {
		return notebookState{state: ContainerStateRunning}
	}

	// Check for .status.containerState.waiting
	if waiting, _ := jq.Query[map[string]any](nb, ".status.containerState.waiting"); waiting != nil {
		reason, _ := waiting["reason"].(string)

		return notebookState{state: ContainerStateWaiting, waitReason: reason}
	}

	// No containerState match (e.g., pod not yet created, or briefly in "terminated"
	// state before StatefulSet restarts it) — treat as waiting.
	return notebookState{state: ContainerStateWaiting}
}

// analyzeNonStoppedWorkloads classifies non-stopped notebooks and builds the diagnostic result.
func (c *NonStoppedWorkloadsCheck) analyzeNonStoppedWorkloads(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) error {
	notebooks := req.Items

	if len(notebooks) == 0 {
		req.Result.SetCondition(check.NewCondition(
			ConditionTypeNonStoppedWorkloads,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgAllNotebooksStopped),
		))

		return nil
	}

	var runningCount, waitingCount int

	impacted := make([]metav1.PartialObjectMetadata, 0, len(notebooks))

	for _, nb := range notebooks {
		state := classifyNotebook(nb)

		annotations := map[string]string{
			AnnotationCheckContainerState: state.state,
		}

		if state.waitReason != "" {
			annotations[AnnotationCheckContainerWaitReason] = state.waitReason
		}

		switch state.state {
		case ContainerStateRunning:
			runningCount++
		case ContainerStateWaiting:
			waitingCount++
		}

		impacted = append(impacted, metav1.PartialObjectMetadata{
			TypeMeta: resources.Notebook.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Name:        nb.GetName(),
				Namespace:   nb.GetNamespace(),
				Annotations: annotations,
			},
		})
	}

	req.Result.ImpactedObjects = impacted
	req.Result.Annotations[result.AnnotationResourceCRDName] = resources.Notebook.CRDFQN()

	// Build summary message.
	var msgParts []string
	msgParts = append(msgParts, fmt.Sprintf(MsgNonStoppedNotebooksFound, len(notebooks)))

	if runningCount > 0 {
		msgParts = append(msgParts, fmt.Sprintf(MsgNonStoppedRunning, runningCount))
	}

	if waitingCount > 0 {
		msgParts = append(msgParts, fmt.Sprintf(MsgNonStoppedWaiting, waitingCount))
	}

	req.Result.SetCondition(check.NewCondition(
		ConditionTypeNonStoppedWorkloads,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage("%s", strings.Join(msgParts, "\n")),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	))

	return nil
}

// SetNamespaceRequesters sets the namespace-to-requester mapping.
// Called by the output renderer before FormatVerboseOutput.
func (c *NonStoppedWorkloadsCheck) SetNamespaceRequesters(m map[string]string) {
	c.namespaceRequesters = m
}

// FormatVerboseOutput implements check.VerboseOutputFormatter.
// Groups non-stopped notebooks by state (running/waiting), then by waiting reason,
// then by namespace within each group.
//
// Output format:
//
//	running (N notebooks):
//	  namespace: <ns> | requester: <email>
//	    - notebooks.kubeflow.org/<name>
//
//	waiting (N notebooks):
//	  <reason> (N):
//	    namespace: <ns> | requester: <email>
//	      - notebooks.kubeflow.org/<name>
func (c *NonStoppedWorkloadsCheck) FormatVerboseOutput(out io.Writer, dr *result.DiagnosticResult) {
	crdName := check.CRDFullyQualifiedName(dr)

	var running []metav1.PartialObjectMetadata
	waitingByReason := make(map[string][]metav1.PartialObjectMetadata)

	for _, obj := range dr.ImpactedObjects {
		state := obj.Annotations[AnnotationCheckContainerState]

		switch state {
		case ContainerStateRunning:
			running = append(running, obj)
		case ContainerStateWaiting:
			reason := obj.Annotations[AnnotationCheckContainerWaitReason]
			if reason == "" {
				reason = "Unknown"
			}

			waitingByReason[reason] = append(waitingByReason[reason], obj)
		}
	}

	if len(running) > 0 {
		_, _ = fmt.Fprintf(out, "    running (%d notebooks):\n", len(running))
		writeNamespaceGroups(out, crdName, running, "      ", c.namespaceRequesters)
		_, _ = fmt.Fprintln(out)
	}

	if len(waitingByReason) > 0 {
		var totalWaiting int
		for _, objs := range waitingByReason {
			totalWaiting += len(objs)
		}

		_, _ = fmt.Fprintf(out, "    waiting (%d notebooks):\n", totalWaiting)

		// Sort reasons for deterministic output.
		reasons := make([]string, 0, len(waitingByReason))
		for reason := range waitingByReason {
			reasons = append(reasons, reason)
		}

		sort.Strings(reasons)

		for _, reason := range reasons {
			objs := waitingByReason[reason]
			_, _ = fmt.Fprintf(out, "      %s (%d):\n", reason, len(objs))
			writeNamespaceGroups(out, crdName, objs, "        ", c.namespaceRequesters)
			_, _ = fmt.Fprintln(out)
		}
	}
}

// writeNamespaceGroups writes notebook references grouped by namespace.
// When requesters is non-nil, the requester annotation is included in the namespace header.
func writeNamespaceGroups(
	out io.Writer,
	crdName string,
	objects []metav1.PartialObjectMetadata,
	indent string,
	requesters map[string]string,
) {
	nsMap := make(map[string][]string)

	for _, obj := range objects {
		nsMap[obj.Namespace] = append(nsMap[obj.Namespace], obj.Name)
	}

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}

	sort.Strings(namespaces)

	for _, ns := range namespaces {
		names := nsMap[ns]
		sort.Strings(names)

		nsHeader := "namespace: " + ns
		if requesters != nil {
			if requester, ok := requesters[ns]; ok && requester != "" {
				nsHeader = fmt.Sprintf("namespace: %s | requester: %s", ns, requester)
			}
		}

		_, _ = fmt.Fprintf(out, "%s%s\n", indent, nsHeader)

		for _, name := range names {
			_, _ = fmt.Fprintf(out, "%s  - %s/%s\n", indent, crdName, name)
		}
	}
}
