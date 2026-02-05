package main

import (
	"os"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/lburgazzoli/odh-cli/cmd/lint"
	"github.com/lburgazzoli/odh-cli/cmd/version"
)

func main() {
	flags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()

	cmd := &cobra.Command{
		Use:   "kubectl-odh",
		Short: "kubectl plugin for ODH/RHOAI",
	}

	// Add kubectl-style flags to root command (inherited by subcommands).
	// This exposes standard authentication flags: --server, --username, --password,
	// --token, --kubeconfig, --context, --cluster, --certificate-authority,
	// --client-certificate, --client-key, --insecure-skip-tls-verify, etc.
	flags.AddFlags(cmd.PersistentFlags())

	version.AddCommand(cmd, flags)
	lint.AddCommand(cmd, flags)

	if err := cmd.Execute(); err != nil {
		if _, writeErr := os.Stderr.WriteString(err.Error() + "\n"); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}
