package color

import "github.com/fatih/color"

// StatusPass returns a green checkmark symbol.
func StatusPass() string {
	return color.New(color.FgGreen).Sprint("✓")
}

// StatusWarn returns a yellow warning symbol.
func StatusWarn() string {
	return color.New(color.FgYellow).Sprint("⚠")
}

// StatusFail returns a red cross symbol.
func StatusFail() string {
	return color.New(color.FgRed).Sprint("✗")
}

// Severity level formatting.

// SeverityCritical returns "critical" in red.
func SeverityCritical() string {
	return color.New(color.FgRed).Sprint("critical")
}

// SeverityWarning returns "warning" in bold yellow.
func SeverityWarning() string {
	return color.New(color.FgYellow).Add(color.Bold).Sprint("warning")
}

// SeverityInfo returns "info" in cyan.
func SeverityInfo() string {
	return color.New(color.FgCyan).Sprint("info")
}

// VerdictFail returns "FAIL" in bold red.
func VerdictFail() string {
	return color.New(color.FgRed, color.Bold).Sprint("FAIL")
}

// VerdictWarning returns "WARNING" in bold yellow.
func VerdictWarning() string {
	return color.New(color.FgYellow, color.Bold).Sprint("WARNING")
}

// VerdictPass returns "PASS" in bold green.
func VerdictPass() string {
	return color.New(color.FgGreen, color.Bold).Sprint("PASS")
}
