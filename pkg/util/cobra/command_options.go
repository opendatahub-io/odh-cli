// Package cobra provides shared registration-time options for Cobra
// commands, such as visibility control.
package cobra

import (
	"github.com/spf13/cobra"

	"github.com/opendatahub-io/odh-cli/pkg/util"
)

// CommandOptions holds the registration-time configuration applied to a
// Cobra command.
type CommandOptions struct {
	// Hidden controls whether the command is excluded from help output,
	// shell completion, and the api manifest. Hiding is a presentation
	// concern only: hidden commands remain directly invocable.
	Hidden bool
}

// CommandOption is a registration option for CommandOptions. Both
// function-based options (see WithHidden) and CommandOptions values,
// which implement the interface themselves, can be mixed freely.
type CommandOption = util.Option[CommandOptions]

// ApplyTo implements CommandOption so a whole CommandOptions struct can
// be passed as an option, optionally bundled with function options.
// Hidden is copied unconditionally: a zero-valued struct explicitly
// unhides its target rather than preserving its current visibility.
func (opts CommandOptions) ApplyTo(target *CommandOptions) {
	target.Hidden = opts.Hidden
}

// WithHidden returns a CommandOption that sets command visibility.
func WithHidden(hidden bool) CommandOption {
	return util.FunctionalOption[CommandOptions](func(opts *CommandOptions) {
		opts.Hidden = hidden
	})
}

// ApplyOptions applies opts to command. Visibility is seeded from the
// command's current state, so supplying no options preserves it. Options
// are applied in order and the last one wins. Only command itself is
// affected; its parent and descendants are left untouched.
func ApplyOptions(command *cobra.Command, opts ...CommandOption) {
	config := CommandOptions{Hidden: command.Hidden}

	util.ApplyOptions(&config, opts...)

	command.Hidden = config.Hidden
}
