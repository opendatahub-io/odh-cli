package deps

// StdinInput defines the JSON/YAML schema for stdin input to deps install.
// Use with --from-stdin to pass configuration via stdin. Precedence: CLI flags > stdin values > defaults.
type StdinInput struct {
	// Deps is the list of dependency names to install.
	// nil = not provided (bulk mode); []string{} = explicitly empty (install nothing).
	// Do not use len(Deps) == 0 to gate bulk mode — that collapses both cases.
	Deps []string `json:"deps" yaml:"deps"`

	// Version is the ODH/RHOAI version to install dependencies for.
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// DryRun shows what would be installed without executing (replaces --dry-run flag).
	// Only true is meaningful; false is equivalent to omitting the field.
	DryRun bool `json:"dryRun,omitempty" yaml:"dryRun,omitempty"`

	// IncludeOptional installs optional dependencies as well (replaces --include-optional flag).
	// Only true is meaningful; false is equivalent to omitting the field.
	IncludeOptional bool `json:"includeOptional,omitempty" yaml:"includeOptional,omitempty"`

	// Refresh fetches the latest manifest from odh-gitops (replaces --refresh flag).
	// Only true is meaningful; false is equivalent to omitting the field.
	Refresh bool `json:"refresh,omitempty" yaml:"refresh,omitempty"`

	// ServiceAccount names a preconfigured ServiceAccount in each operator namespace for OLM v1.
	ServiceAccount string `json:"serviceAccount,omitempty" yaml:"serviceAccount,omitempty"`

	// OLMMode selects the installation API when both OLM versions are available.
	OLMMode string `json:"olmMode,omitempty" yaml:"olmMode,omitempty"`
}
