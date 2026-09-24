package status

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	"github.com/opendatahub-io/odh-cli/pkg/deps"
	utilcolor "github.com/opendatahub-io/odh-cli/pkg/util/color"
)

const (
	colWidthSection = 12
	colWidthStatus  = 6
	maxSummaryLen   = 60
	maxDetailLen    = 50

	msgInsufficientPerms = "insufficient permissions"
)

// sectionDisplayOrder defines the order and display names for sections.
//
//nolint:gochecknoglobals // Package-level constant data for section ordering
var sectionDisplayOrder = []struct {
	key     string
	display string
}{
	{clusterhealth.SectionNodes, "Nodes"},
	{clusterhealth.SectionDeployments, "Deployments"},
	{clusterhealth.SectionPods, "Pods"},
	{clusterhealth.SectionEvents, "Events"},
	{clusterhealth.SectionQuotas, "Quotas"},
	{clusterhealth.SectionOperator, "Operator"},
	{clusterhealth.SectionDSCI, "DSCI"},
	{clusterhealth.SectionDSC, "DSC"},
}

// isRBACError checks if the error string indicates a permission/RBAC issue.
// Allocates once for the lowercased string, then checks all patterns.
func isRBACError(errStr string) bool {
	if errStr == "" {
		return false
	}

	lower := strings.ToLower(errStr)

	return strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "cannot list") ||
		strings.Contains(lower, "cannot get")
}

// statusSymbol returns the appropriate colored symbol for a section.
func statusSymbol(errStr string) string {
	if errStr == "" {
		return utilcolor.StatusPass()
	}
	if isRBACError(errStr) {
		return utilcolor.StatusUnknown()
	}

	return utilcolor.StatusFail()
}

// ansiEscapeRegex matches ANSI escape codes for color calculation.
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visibleLen returns the visible length of a string, excluding ANSI codes.
func visibleLen(s string) int {
	return utf8.RuneCountInString(ansiEscapeRegex.ReplaceAllString(s, ""))
}

// padRight pads a string to the specified visible width.
func padRight(s string, width int) string {
	visible := visibleLen(s)
	if visible >= width {
		return s
	}

	return s + strings.Repeat(" ", width-visible)
}

// truncate shortens a string to maxLen runes, adding "..." if truncated.
// Uses rune-aware slicing to avoid corrupting multi-byte UTF-8 characters.
func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	if maxLen <= 3 {
		return "..."
	}

	return string(runes[:maxLen-3]) + "..."
}

// renderTableOutput renders the report as a formatted table with symbols.
func (c *Command) renderTableOutput(report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	w := c.IO.Out()

	// Build section filter set
	runSet := make(map[string]bool)
	for _, k := range report.SectionsRun {
		runSet[k] = true
	}

	// Print table header
	if err := writeTableHeader(w); err != nil {
		return err
	}

	// Print each section row
	for _, sec := range sectionDisplayOrder {
		if len(runSet) > 0 && !runSet[sec.key] {
			continue
		}

		errStr, summary := c.getSectionData(report, sec.key)
		symbol := statusSymbol(errStr)

		if err := writeTableRow(w, sec.display, symbol, summary); err != nil {
			return err
		}
	}

	// Print verbose details if requested
	if c.Verbose {
		if err := c.renderVerboseDetails(w, report, runSet); err != nil {
			return err
		}
	}

	// Print dependencies if available
	if len(depStatuses) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return fmt.Errorf("writing newline: %w", err)
		}
		if err := c.renderDependenciesTable(w, depStatuses); err != nil {
			return err
		}
	}

	return nil
}

func writeTableHeader(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "%s  %s  SUMMARY\n",
		padRight("SECTION", colWidthSection),
		padRight("STATUS", colWidthStatus)); err != nil {
		return fmt.Errorf("writing table header: %w", err)
	}

	if _, err := fmt.Fprintf(w, "%s  %s  %s\n",
		strings.Repeat("-", colWidthSection),
		strings.Repeat("-", colWidthStatus),
		"-------"); err != nil {
		return fmt.Errorf("writing table separator: %w", err)
	}

	return nil
}

func writeTableRow(w io.Writer, section, symbol, summary string) error {
	if _, err := fmt.Fprintf(w, "%s  %s  %s\n",
		padRight(section, colWidthSection),
		padRight(symbol, colWidthStatus),
		summary); err != nil {
		return fmt.Errorf("writing table row: %w", err)
	}

	return nil
}

// getSectionData returns the error string and summary for a section.
//
//nolint:nonamedreturns // Named returns would be redundant here
func (c *Command) getSectionData(report *clusterhealth.Report, key string) (errStr string, summary string) {
	switch key {
	case clusterhealth.SectionNodes:
		return report.Nodes.Error, c.summaryNodes(report)
	case clusterhealth.SectionDeployments:
		return report.Deployments.Error, c.summaryDeployments(report)
	case clusterhealth.SectionPods:
		return report.Pods.Error, c.summaryPods(report)
	case clusterhealth.SectionEvents:
		return report.Events.Error, c.summaryEvents(report)
	case clusterhealth.SectionQuotas:
		return report.Quotas.Error, c.summaryQuotas(report)
	case clusterhealth.SectionOperator:
		return report.Operator.Error, c.summaryOperator(report)
	case clusterhealth.SectionDSCI:
		return report.DSCI.Error, c.summaryDSCI(report)
	case clusterhealth.SectionDSC:
		return report.DSC.Error, c.summaryDSC(report)
	default:
		return "", ""
	}
}

// Section summary functions

func (c *Command) summaryNodes(r *clusterhealth.Report) string {
	if r.Nodes.Error != "" {
		if isRBACError(r.Nodes.Error) {
			return msgInsufficientPerms
		}

		return truncate(r.Nodes.Error, maxSummaryLen)
	}

	n := len(r.Nodes.Data.Nodes)
	if n == 0 {
		return "no nodes"
	}

	unhealthy := 0
	for _, node := range r.Nodes.Data.Nodes {
		if node.UnhealthyReason != "" {
			unhealthy++
		}
	}
	if unhealthy == 0 {
		return fmt.Sprintf("%d nodes", n)
	}

	return fmt.Sprintf("%d nodes, %d unhealthy", n, unhealthy)
}

func (c *Command) summaryDeployments(r *clusterhealth.Report) string {
	if r.Deployments.Error != "" {
		if isRBACError(r.Deployments.Error) {
			return msgInsufficientPerms
		}

		return truncate(r.Deployments.Error, maxSummaryLen)
	}

	byNs := r.Deployments.Data.ByNamespace
	if len(byNs) == 0 {
		return "no deployments"
	}

	total, notReady, nsCount := 0, 0, 0
	for _, infos := range byNs {
		if len(infos) > 0 {
			nsCount++
		}
		for _, d := range infos {
			total++
			if d.Replicas > 0 && d.Ready != d.Replicas {
				notReady++
			}
		}
	}
	if notReady == 0 {
		return fmt.Sprintf("%d deployments in %d namespaces", total, nsCount)
	}

	return fmt.Sprintf("%d deployments in %d namespaces, %d not ready", total, nsCount, notReady)
}

func (c *Command) summaryPods(r *clusterhealth.Report) string {
	if r.Pods.Error != "" {
		if isRBACError(r.Pods.Error) {
			return msgInsufficientPerms
		}

		return truncate(r.Pods.Error, maxSummaryLen)
	}

	byNs := r.Pods.Data.ByNamespace
	if len(byNs) == 0 {
		return "no pods"
	}

	total, notRunning, nsCount := 0, 0, 0
	for _, infos := range byNs {
		if len(infos) > 0 {
			nsCount++
		}
		for _, p := range infos {
			total++
			if p.Phase != "Running" {
				notRunning++
			}
		}
	}
	if notRunning == 0 {
		return fmt.Sprintf("%d pods in %d namespaces", total, nsCount)
	}

	return fmt.Sprintf("%d pods in %d namespaces, %d not Running", total, nsCount, notRunning)
}

func (c *Command) summaryEvents(r *clusterhealth.Report) string {
	if r.Events.Error != "" {
		if isRBACError(r.Events.Error) {
			return msgInsufficientPerms
		}

		return truncate(r.Events.Error, maxSummaryLen)
	}

	n := len(r.Events.Data.Events)
	if n == 0 {
		return "no events"
	}

	return fmt.Sprintf("%d events", n)
}

func (c *Command) summaryQuotas(r *clusterhealth.Report) string {
	if r.Quotas.Error != "" {
		if isRBACError(r.Quotas.Error) {
			return msgInsufficientPerms
		}

		return truncate(r.Quotas.Error, maxSummaryLen)
	}

	byNs := r.Quotas.Data.ByNamespace
	if len(byNs) == 0 {
		return "no quotas"
	}

	total, exceeded, nsCount := 0, 0, 0
	for _, infos := range byNs {
		if len(infos) > 0 {
			nsCount++
		}
		for _, q := range infos {
			total++
			if len(q.Exceeded) > 0 {
				exceeded++
			}
		}
	}
	if exceeded == 0 {
		return fmt.Sprintf("%d quotas in %d namespaces", total, nsCount)
	}

	return fmt.Sprintf("%d quotas in %d namespaces, %d exceeded", total, nsCount, exceeded)
}

func (c *Command) summaryOperator(r *clusterhealth.Report) string {
	if r.Operator.Error != "" {
		if isRBACError(r.Operator.Error) {
			return msgInsufficientPerms
		}

		return truncate(r.Operator.Error, maxSummaryLen)
	}

	d := r.Operator.Data.Deployment
	if d == nil {
		return "no operator deployment"
	}

	depLine := fmt.Sprintf("%s %d/%d ready", d.Name, d.Ready, d.Replicas)
	dependents := r.Operator.Data.DependentOperators
	if len(dependents) == 0 {
		return depLine
	}

	notInstalled := 0
	for _, dep := range dependents {
		if !dep.Installed {
			notInstalled++
		}
	}
	if notInstalled == 0 {
		return fmt.Sprintf("%s, %d dependents", depLine, len(dependents))
	}

	return fmt.Sprintf("%s, %d dependents (%d not installed)", depLine, len(dependents), notInstalled)
}

func (c *Command) summaryDSCI(r *clusterhealth.Report) string {
	if r.DSCI.Error != "" {
		if isRBACError(r.DSCI.Error) {
			return msgInsufficientPerms
		}

		return truncate(r.DSCI.Error, maxSummaryLen)
	}

	name := r.DSCI.Data.Name
	if name == "" {
		return "no DSCI"
	}

	n := len(r.DSCI.Data.Conditions)
	if n == 0 {
		return name
	}

	return fmt.Sprintf("%s, %d conditions", name, n)
}

func (c *Command) summaryDSC(r *clusterhealth.Report) string {
	if r.DSC.Error != "" {
		if isRBACError(r.DSC.Error) {
			return msgInsufficientPerms
		}

		return truncate(r.DSC.Error, maxSummaryLen)
	}

	name := r.DSC.Data.Name
	if name == "" {
		return "no DSC"
	}

	n := len(r.DSC.Data.Conditions)
	if n == 0 {
		return name
	}

	return fmt.Sprintf("%s, %d conditions", name, n)
}

// renderVerboseDetails outputs expanded details for each section.
func (c *Command) renderVerboseDetails(w io.Writer, report *clusterhealth.Report, runSet map[string]bool) error {
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("writing newline: %w", err)
	}
	if _, err := fmt.Fprintln(w, "--- Details ---"); err != nil {
		return fmt.Errorf("writing details header: %w", err)
	}

	for _, sec := range sectionDisplayOrder {
		if len(runSet) > 0 && !runSet[sec.key] {
			continue
		}

		details := c.getVerboseDetails(report, sec.key)
		if details != "" {
			if _, err := fmt.Fprintf(w, "%s:\n%s\n", sec.display, details); err != nil {
				return fmt.Errorf("writing section details: %w", err)
			}
		}
	}

	return nil
}

func (c *Command) getVerboseDetails(r *clusterhealth.Report, key string) string {
	switch key {
	case clusterhealth.SectionNodes:
		return c.verboseNodes(r)
	case clusterhealth.SectionDeployments:
		return c.verboseDeployments(r)
	case clusterhealth.SectionPods:
		return c.verbosePods(r)
	case clusterhealth.SectionEvents:
		return c.verboseEvents(r)
	case clusterhealth.SectionQuotas:
		return c.verboseQuotas(r)
	case clusterhealth.SectionOperator:
		return c.verboseOperator(r)
	case clusterhealth.SectionDSCI:
		return c.verboseCRConditions(r.DSCI.Data.Name, r.DSCI.Data.Conditions, r.DSCI.Error)
	case clusterhealth.SectionDSC:
		return c.verboseCRConditions(r.DSC.Data.Name, r.DSC.Data.Conditions, r.DSC.Error)
	default:
		return ""
	}
}

func (c *Command) verboseNodes(r *clusterhealth.Report) string {
	if r.Nodes.Error != "" {
		return "  " + r.Nodes.Error
	}

	var b strings.Builder
	for _, n := range r.Nodes.Data.Nodes {
		nodeStatus := "OK"
		if n.UnhealthyReason != "" {
			nodeStatus = n.UnhealthyReason
		}
		_, _ = fmt.Fprintf(&b, "  %s: %s\n", n.Name, nodeStatus)
	}

	return b.String()
}

func (c *Command) verboseDeployments(r *clusterhealth.Report) string {
	if r.Deployments.Error != "" {
		return "  " + r.Deployments.Error
	}

	var b strings.Builder
	for ns, infos := range r.Deployments.Data.ByNamespace {
		for _, d := range infos {
			_, _ = fmt.Fprintf(&b, "  %s/%s %d/%d ready\n", ns, d.Name, d.Ready, d.Replicas)
		}
	}

	return b.String()
}

func (c *Command) verbosePods(r *clusterhealth.Report) string {
	if r.Pods.Error != "" {
		return "  " + r.Pods.Error
	}

	var b strings.Builder
	for ns, infos := range r.Pods.Data.ByNamespace {
		for _, p := range infos {
			_, _ = fmt.Fprintf(&b, "  %s/%s %s\n", ns, p.Name, p.Phase)
		}
	}

	return b.String()
}

func (c *Command) verboseEvents(r *clusterhealth.Report) string {
	if r.Events.Error != "" {
		return "  " + r.Events.Error
	}

	var b strings.Builder
	for _, e := range r.Events.Data.Events {
		_, _ = fmt.Fprintf(&b, "  %s/%s %s: %s\n", e.Namespace, e.Name, e.Reason, truncate(e.Message, maxDetailLen))
	}

	return b.String()
}

func (c *Command) verboseQuotas(r *clusterhealth.Report) string {
	if r.Quotas.Error != "" {
		return "  " + r.Quotas.Error
	}

	var b strings.Builder
	for ns, infos := range r.Quotas.Data.ByNamespace {
		for _, q := range infos {
			_, _ = fmt.Fprintf(&b, "  %s/%s", ns, q.Name)
			if len(q.Exceeded) > 0 {
				_, _ = fmt.Fprintf(&b, " exceeded: %s", strings.Join(q.Exceeded, ", "))
			}
			_, _ = fmt.Fprintln(&b)
		}
	}

	return b.String()
}

func (c *Command) verboseOperator(r *clusterhealth.Report) string {
	if r.Operator.Error != "" {
		return "  " + r.Operator.Error
	}

	var b strings.Builder
	d := r.Operator.Data.Deployment
	if d != nil {
		_, _ = fmt.Fprintf(&b, "  %s %d/%d ready\n", d.Name, d.Ready, d.Replicas)
	}

	for _, dep := range r.Operator.Data.DependentOperators {
		_, _ = fmt.Fprintf(&b, "  dependent: %s", dep.Name)

		switch {
		case !dep.Installed:
			_, _ = fmt.Fprint(&b, " not installed")
		case dep.Error != "":
			_, _ = fmt.Fprintf(&b, " error: %s", dep.Error)
		case dep.Deployment != nil:
			_, _ = fmt.Fprintf(&b, " %d/%d ready", dep.Deployment.Ready, dep.Deployment.Replicas)
		}
		_, _ = fmt.Fprintln(&b)
	}

	return b.String()
}

func (c *Command) verboseCRConditions(name string, conditions []clusterhealth.ConditionSummary, errStr string) string {
	if errStr != "" {
		return "  " + errStr
	}
	if name == "" {
		return ""
	}

	var b strings.Builder
	for _, cond := range conditions {
		_, _ = fmt.Fprintf(&b, "  %s: %s", cond.Type, cond.Status)
		if cond.Message != "" {
			_, _ = fmt.Fprintf(&b, " (%s)", truncate(cond.Message, maxDetailLen))
		}
		_, _ = fmt.Fprintln(&b)
	}

	return b.String()
}

// renderDependenciesTable outputs dependency status.
func (c *Command) renderDependenciesTable(w io.Writer, statuses []deps.DependencyStatus) error {
	if _, err := fmt.Fprintln(w, "Dependencies:"); err != nil {
		return fmt.Errorf("writing dependencies header: %w", err)
	}

	for _, dep := range statuses {
		var symbol string

		if dep.Error != "" {
			symbol = utilcolor.StatusWarn()
		} else {
			symbol = dependencyStatusSymbol(dep.Status)
		}

		line := fmt.Sprintf("  %s %s", symbol, dep.DisplayName)

		if dep.Version != "" {
			line += fmt.Sprintf(" (%s)", dep.Version)
		}

		if dep.Error != "" {
			line += " - error: " + dep.Error
		} else if dep.Status == deps.StatusMissing && len(dep.RequiredBy) > 0 {
			line += " - required by: " + strings.Join(dep.RequiredBy, ", ")
		}

		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("writing dependency line: %w", err)
		}
	}

	return nil
}

// dependencyStatusSymbol returns the appropriate symbol for a dependency status.
func dependencyStatusSymbol(status deps.Status) string {
	switch status {
	case deps.StatusInstalled:
		return utilcolor.StatusPass()
	case deps.StatusMissing:
		return utilcolor.StatusFail()
	case deps.StatusOptional, deps.StatusPending:
		return utilcolor.StatusWarn()
	case deps.StatusUnknown:
		return utilcolor.StatusUnknown()
	}

	return utilcolor.StatusUnknown()
}
