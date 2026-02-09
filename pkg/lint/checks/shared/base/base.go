package base

import (
	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
)

// BaseCheck provides common check metadata and functionality through composition.
// Embed this struct in check implementations to eliminate boilerplate code.
//
// Example usage:
//
//	type RemovalCheck struct {
//	    base.BaseCheck
//	}
//
//	func NewRemovalCheck() *RemovalCheck {
//	    return &RemovalCheck{
//	        BaseCheck: base.BaseCheck{
//	            CheckGroup:       check.GroupComponent,
//	            Kind:             check.ComponentModelMesh,
//	            Type:             check.CheckTypeRemoval,
//	            CheckID:          "components.modelmesh.removal",
//	            CheckName:        "Components :: ModelMesh :: Removal (3.x)",
//	            CheckDescription: "Validates that ModelMesh is disabled...",
//	            CheckRemediation: "",
//	        },
//	    }
//	}
type BaseCheck struct {
	CheckGroup       check.CheckGroup
	Kind             string
	Type             string
	CheckID          string
	CheckName        string
	CheckDescription string
	CheckRemediation string
}

// ID returns the unique identifier for this check.
// Required by check.Check interface.
func (b BaseCheck) ID() string {
	return b.CheckID
}

// Name returns the human-readable check name.
// Required by check.Check interface.
func (b BaseCheck) Name() string {
	return b.CheckName
}

// Description returns what this check validates.
// Required by check.Check interface.
func (b BaseCheck) Description() string {
	return b.CheckDescription
}

// Remediation returns guidance on how to fix issues found by this check.
func (b BaseCheck) Remediation() string {
	return b.CheckRemediation
}

// Group returns the check group.
// Required by check.Check interface.
func (b BaseCheck) Group() check.CheckGroup {
	return b.CheckGroup
}

// CheckKind returns the kind of resource being checked.
// Required by check.Check interface.
func (b BaseCheck) CheckKind() string {
	return b.Kind
}

// CheckType returns the type of check (e.g., "removal", "deprecation").
// Required by check.Check interface.
func (b BaseCheck) CheckType() string {
	return b.Type
}

// NewResult creates a DiagnosticResult initialized with this check's metadata.
// This is the primary convenience method that eliminates result.New() boilerplate.
//
// Example:
//
//	func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
//	    dr := c.NewResult()  // Instead of result.New(...)
//
//	    // Add conditions...
//	    return dr, nil
//	}
func (b BaseCheck) NewResult() *result.DiagnosticResult {
	return result.New(
		string(b.CheckGroup),
		b.Kind,
		b.Type,
		b.CheckDescription,
	)
}
