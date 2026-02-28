package lint

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/fatih/color"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/printer/table"
)

//nolint:gochecknoglobals
var (
	// Table output symbols.
	statusPass = color.New(color.FgGreen).Sprint("✓")
	statusWarn = color.New(color.FgYellow).Sprint("⚠")
	statusFail = color.New(color.FgRed).Sprint("✗")

	// Severity level formatting.
	severityCrit = color.New(color.FgRed).Sprint("critical")
	severityWarn = color.New(color.FgYellow).Add(color.Bold).Sprint("warning") // Bold yellow (orange-ish)
	severityInfo = color.New(color.FgCyan).Sprint("info")

	// Table headers.
	tableHeaders        = []string{"STATUS", "KIND", "GROUP", "CHECK", "IMPACT", "MESSAGE"}
	verboseTableHeaders = []string{"STATUS", "KIND", "GROUP", "CHECK", "IMPACT"}

	// ansiEscapeRegex matches ANSI escape sequences for stripping when computing visible width.
	ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

// printVerdict prints the Result section after the summary.
func printVerdict(out io.Writer, hasBlocking bool, hasAdvisory bool) {
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Result:")

	switch {
	case hasBlocking:
		verdict := color.New(color.FgRed, color.Bold).Sprint("FAIL")
		_, _ = fmt.Fprintf(out, "  %s - blocking findings detected\n", verdict)
	case hasAdvisory:
		verdict := color.New(color.FgYellow, color.Bold).Sprint("WARNING")
		_, _ = fmt.Fprintf(out, "  %s - advisory findings detected\n", verdict)
	default:
		verdict := color.New(color.FgGreen, color.Bold).Sprint("PASS")
		_, _ = fmt.Fprintf(out, "  %s - all checks passed\n", verdict)
	}
}

// sortableRow pairs a table row with the raw impact for sort comparisons.
type sortableRow struct {
	row    CheckResultTableRow
	impact result.Impact
}

// collectSortedRows builds table rows from check executions and sorts them
// by Group (canonical) -> Kind -> Impact (critical, warning, info) -> Check.
func collectSortedRows(results []check.CheckExecution) []sortableRow {
	totalConditions := 0
	for _, exec := range results {
		if exec.Result == nil {
			continue
		}

		totalConditions += len(exec.Result.Status.Conditions)
	}

	rows := make([]sortableRow, 0, totalConditions)

	for _, exec := range results {
		if exec.Result == nil {
			continue
		}

		for _, condition := range exec.Result.Status.Conditions {
			rows = append(rows, sortableRow{
				row: CheckResultTableRow{
					Status:      statusSymbol(condition.Impact),
					Kind:        exec.Result.Kind,
					Group:       exec.Result.Group,
					Check:       exec.Result.Name,
					Impact:      getImpactString(&condition, severityCrit, severityWarn, severityInfo),
					Message:     condition.Message,
					Description: exec.Result.Spec.Description,
				},
				impact: condition.Impact,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		gi, gj := groupSortPriority(rows[i].row.Group), groupSortPriority(rows[j].row.Group)
		if gi != gj {
			return gi < gj
		}

		if rows[i].row.Kind != rows[j].row.Kind {
			return rows[i].row.Kind < rows[j].row.Kind
		}

		pi, pj := impactSortPriority(rows[i].impact), impactSortPriority(rows[j].impact)
		if pi != pj {
			return pi < pj
		}

		return rows[i].row.Check < rows[j].row.Check
	})

	return rows
}

// statusSymbol returns the colored status symbol for the given impact level.
func statusSymbol(impact result.Impact) string {
	switch impact {
	case result.ImpactBlocking:
		return statusFail
	case result.ImpactAdvisory:
		return statusWarn
	case result.ImpactNone:
		return statusPass
	}

	return statusPass
}

// visibleLen returns the display width (rune count) of a string after stripping
// ANSI escape sequences. This gives the correct terminal column width for strings
// containing multi-byte Unicode characters (✓, ⚠, ✗) and ANSI color codes.
func visibleLen(s string) int {
	return utf8.RuneCountInString(ansiEscapeRegex.ReplaceAllString(s, ""))
}

// padRight pads a string (which may contain ANSI codes and multi-byte Unicode)
// to the given visible width. Since fmt uses rune count for width, we compute the
// target rune count that produces the desired display width.
func padRight(s string, visibleWidth int) string {
	pad := visibleWidth - visibleLen(s) + utf8.RuneCountInString(s)

	return fmt.Sprintf("%-*s", pad, s)
}

// OutputTable is a shared function for outputting check results in table format.
// When opts.ShowImpactedObjects is true, impacted objects are listed after the summary.
func OutputTable(out io.Writer, results []check.CheckExecution, opts TableOutputOptions) error {
	rows := collectSortedRows(results)

	renderer := table.NewRenderer[CheckResultTableRow](
		table.WithWriter[CheckResultTableRow](out),
		table.WithHeaders[CheckResultTableRow](tableHeaders...),
		table.WithTableOptions[CheckResultTableRow](table.DefaultTableOptions...),
	)

	totalChecks := 0
	totalPassed := 0
	totalWarnings := 0
	totalFailed := 0

	for _, sr := range rows {
		totalChecks++

		switch sr.impact {
		case result.ImpactBlocking:
			totalFailed++
		case result.ImpactAdvisory:
			totalWarnings++
		case result.ImpactNone:
			totalPassed++
		}

		if err := renderer.Append(sr.row); err != nil {
			return fmt.Errorf("appending table row: %w", err)
		}
	}

	if err := renderer.Render(); err != nil {
		return fmt.Errorf("rendering table: %w", err)
	}

	if opts.VersionInfo != nil {
		_, _ = fmt.Fprintln(out)
		outputVersionInfo(out, opts.VersionInfo)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Summary:")
	_, _ = fmt.Fprintf(out, "  Total: %d | Passed: %d | Warnings: %d | Failed: %d\n", totalChecks, totalPassed, totalWarnings, totalFailed)

	if opts.ShowImpactedObjects {
		outputImpactedObjects(out, results, opts.NamespaceRequesters)
	}

	return nil
}

// outputVersionInfo prints the Environment section with version details.
func outputVersionInfo(out io.Writer, info *VersionInfo) {
	_, _ = fmt.Fprintln(out, "Environment:")

	if info.RHOAITargetVersion != "" {
		_, _ = fmt.Fprintf(out, "  OpenShift AI version: %s -> %s\n", info.RHOAICurrentVersion, info.RHOAITargetVersion)
	} else {
		_, _ = fmt.Fprintf(out, "  OpenShift AI version: %s\n", info.RHOAICurrentVersion)
	}

	if info.OpenShiftVersion != "" {
		_, _ = fmt.Fprintf(out, "  OpenShift version:    %s\n", info.OpenShiftVersion)
	}
}

// namespaceRequesterSetter is implemented by verbose formatters that need
// namespace-to-requester mappings (e.g. NotebookVerboseFormatter).
type namespaceRequesterSetter interface {
	SetNamespaceRequesters(requesters map[string]string)
}

// verboseRow holds a single impacted-objects table entry with pre-rendered detail.
type verboseRow struct {
	status    string
	kind      string
	group     string
	check     string
	impact    string
	exec      check.CheckExecution
	detailBuf bytes.Buffer // pre-rendered verbose detail
}

// borderPadding is the total horizontal padding inside table borders ("│ " + " │").
const borderPadding = 2

// buildVerboseRows filters results to those with impacted objects, pre-renders
// verbose detail, and returns the rows sorted by the canonical check order.
func buildVerboseRows(
	results []check.CheckExecution,
	namespaceRequesters map[string]string,
) []*verboseRow {
	defaultFmt := &check.DefaultVerboseFormatter{
		NamespaceRequesters: namespaceRequesters,
	}

	var rows []*verboseRow

	for _, exec := range results {
		if exec.Result == nil || len(exec.Result.ImpactedObjects) == 0 {
			continue
		}

		maxImpact := checkMaxImpact(exec)
		r := &verboseRow{
			status: statusSymbol(maxImpact),
			kind:   exec.Result.Kind,
			group:  exec.Result.Group,
			check:  exec.Result.Name,
			impact: getImpactString(
				&result.Condition{Impact: maxImpact},
				severityCrit, severityWarn, severityInfo,
			),
			exec: exec,
		}

		// Pre-render verbose detail to a buffer so we can measure line widths.
		if f, ok := exec.Check.(check.VerboseOutputFormatter); ok {
			if nrs, ok := exec.Check.(namespaceRequesterSetter); ok {
				nrs.SetNamespaceRequesters(namespaceRequesters)
			}

			f.FormatVerboseOutput(&r.detailBuf, exec.Result)
		} else {
			defaultFmt.FormatVerboseOutput(&r.detailBuf, exec.Result)
		}

		rows = append(rows, r)
	}

	// Sort rows identically to the summary table: group → kind → impact → check.
	sort.Slice(rows, func(i, j int) bool {
		gi, gj := groupSortPriority(rows[i].group), groupSortPriority(rows[j].group)
		if gi != gj {
			return gi < gj
		}

		if rows[i].kind != rows[j].kind {
			return rows[i].kind < rows[j].kind
		}

		pi, pj := impactSortPriority(checkMaxImpact(rows[i].exec)), impactSortPriority(checkMaxImpact(rows[j].exec))
		if pi != pj {
			return pi < pj
		}

		return rows[i].check < rows[j].check
	})

	return rows
}

// verboseTableLayout holds pre-computed column widths, inner width, and border strings.
type verboseTableLayout struct {
	colWidths       []int
	colContentWidth int
	innerWidth      int
	topBorder       string
	headerSep       string
	bottomBorder    string
}

// computeVerboseLayout calculates column widths and border strings for the verbose table.
func computeVerboseLayout(rows []*verboseRow) verboseTableLayout {
	const colGap = 2

	colWidths := make([]int, len(verboseTableHeaders))
	for i, h := range verboseTableHeaders {
		colWidths[i] = len(h)
	}

	for _, r := range rows {
		for i, v := range []string{r.status, r.kind, r.group, r.check, r.impact} {
			if vl := visibleLen(v); vl > colWidths[i] {
				colWidths[i] = vl
			}
		}
	}

	// Calculate minimum inner width from column layout: columns + gaps + side padding.
	colContentWidth := 1 // leading space
	innerWidth := 0

	for i, w := range colWidths {
		innerWidth += w
		colContentWidth += w

		if i < len(colWidths)-1 {
			innerWidth += colGap
			colContentWidth += colGap
		}
	}

	innerWidth += borderPadding

	// Expand inner width to fit the widest verbose detail line.
	for _, r := range rows {
		for line := range strings.SplitSeq(r.detailBuf.String(), "\n") {
			if lineWidth := visibleLen(line) + borderPadding; lineWidth > innerWidth {
				innerWidth = lineWidth
			}
		}
	}

	hLine := strings.Repeat("─", innerWidth)

	return verboseTableLayout{
		colWidths:       colWidths,
		colContentWidth: colContentWidth,
		innerWidth:      innerWidth,
		topBorder:       "┌" + hLine + "┐",
		headerSep:       "├" + hLine + "┤",
		bottomBorder:    "└" + hLine + "┘",
	}
}

// formatVerboseRow renders a single data row with left/right borders.
func formatVerboseRow(vals []string, layout verboseTableLayout) string {
	var b strings.Builder
	_, _ = b.WriteString("│ ")

	for i, v := range vals {
		if i > 0 {
			_, _ = b.WriteString("  ")
		}

		_, _ = b.WriteString(padRight(v, layout.colWidths[i]))
	}

	if trailing := layout.innerWidth - layout.colContentWidth - 1; trailing > 0 {
		_, _ = b.WriteString(strings.Repeat(" ", trailing))
	}

	_, _ = b.WriteString(" │")

	return b.String()
}

// formatVerboseDetailLine wraps a pre-rendered detail line with left/right borders.
func formatVerboseDetailLine(line string, innerWidth int) string {
	var b strings.Builder
	_, _ = b.WriteString("│ ")
	_, _ = b.WriteString(line)

	if pad := innerWidth - borderPadding - visibleLen(line); pad > 0 {
		_, _ = b.WriteString(strings.Repeat(" ", pad))
	}

	_, _ = b.WriteString(" │")

	return b.String()
}

// outputImpactedObjects prints impacted objects in a bordered 5-column table
// matching the summary table's style (STATUS, KIND, GROUP, CHECK, IMPACT).
// Verbose detail lines from VerboseOutputFormatter appear beneath each data row,
// inside the table borders. The table width is sized to contain the widest
// content line (including verbose detail such as image summary descriptions).
func outputImpactedObjects(
	out io.Writer,
	results []check.CheckExecution,
	namespaceRequesters map[string]string,
) {
	rows := buildVerboseRows(results, namespaceRequesters)
	if len(rows) == 0 {
		return
	}

	layout := computeVerboseLayout(rows)

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Impacted Objects:")
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, layout.topBorder)
	_, _ = fmt.Fprintln(out, formatVerboseRow(verboseTableHeaders, layout))
	_, _ = fmt.Fprintln(out, layout.headerSep)

	for idx, r := range rows {
		vals := []string{r.status, r.kind, r.group, r.check, r.impact}
		_, _ = fmt.Fprintln(out, formatVerboseRow(vals, layout))
		_, _ = fmt.Fprintln(out, formatVerboseDetailLine("", layout.innerWidth))

		detail := strings.TrimRight(r.detailBuf.String(), "\n")
		for line := range strings.SplitSeq(detail, "\n") {
			_, _ = fmt.Fprintln(out, formatVerboseDetailLine(line, layout.innerWidth))
		}

		_, _ = fmt.Fprintln(out, formatVerboseDetailLine("", layout.innerWidth))
		_, _ = fmt.Fprintln(out, formatVerboseDetailLine("", layout.innerWidth))

		if idx < len(rows)-1 {
			_, _ = fmt.Fprintln(out, layout.headerSep)
		}
	}

	_, _ = fmt.Fprintln(out, layout.bottomBorder)
}
