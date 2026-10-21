package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/cmd/api"
	"github.com/opendatahub-io/odh-cli/cmd/completion"
	"github.com/opendatahub-io/odh-cli/cmd/components"
	"github.com/opendatahub-io/odh-cli/cmd/deps"
	"github.com/opendatahub-io/odh-cli/cmd/diagnose"
	"github.com/opendatahub-io/odh-cli/cmd/events"
	"github.com/opendatahub-io/odh-cli/cmd/get"
	"github.com/opendatahub-io/odh-cli/cmd/lint"
	"github.com/opendatahub-io/odh-cli/cmd/logs"
	"github.com/opendatahub-io/odh-cli/cmd/mcp"
	"github.com/opendatahub-io/odh-cli/cmd/migrate"
	"github.com/opendatahub-io/odh-cli/cmd/status"
	"github.com/opendatahub-io/odh-cli/cmd/version"
	internalversion "github.com/opendatahub-io/odh-cli/internal/version"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

// newRootCommand assembles the root command with all subcommands and
// their visibility. Visibility is explicit maturity policy: lint,
// migrate, and version are user-facing; everything else stays hidden
// until it matures. Promote a command by flipping its WithHidden value.
func newRootCommand() *cobra.Command {
	flags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()

	cmd := &cobra.Command{
		Use:     internalversion.GetCLIName(),
		Short:   "kubectl plugin for ODH/RHOAI",
		Version: internalversion.GetVersion(),
	}

	// Add kubectl-style flags to root command (inherited by subcommands).
	// This exposes standard authentication flags: --server, --username, --password,
	// --token, --kubeconfig, --context, --cluster, --certificate-authority,
	// --client-certificate, --client-key, --insecure-skip-tls-verify, etc.
	flags.AddFlags(cmd.PersistentFlags())

	// supported flags
	version.AddCommand(cmd, flags, utilcobra.WithHidden(false))
	lint.AddCommand(cmd, flags, utilcobra.WithHidden(false))
	migrate.AddCommand(cmd, flags, utilcobra.WithHidden(false))

	// hidden/unsupported flags
	api.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	get.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	deps.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	components.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	status.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	logs.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	completion.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	mcp.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	events.AddCommand(cmd, flags, utilcobra.WithHidden(true))
	diagnose.AddCommand(cmd, flags, utilcobra.WithHidden(true))

	return cmd
}

func main() {
	cmd := newRootCommand()

	if err := cmd.Execute(); err != nil {
		exitCode := int(clierrors.ExitCodeFromError(err))

		if !errors.Is(err, clierrors.ErrAlreadyHandled) {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
		}

		os.Exit(exitCode)
	}
}
