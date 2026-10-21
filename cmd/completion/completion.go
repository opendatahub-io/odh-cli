package completion

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
)

const (
	cmdName  = "completion"
	cmdShort = "Generate shell completion scripts"
)

const cmdLong = `Generate shell completion scripts for kubectl-odh.

To load completions:

Bash:
  $ source <(kubectl-odh completion bash)
  # Or for kubectl plugin:
  $ source <(kubectl odh completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ kubectl-odh completion bash > /etc/bash_completion.d/kubectl-odh
  # macOS:
  $ kubectl-odh completion bash > $(brew --prefix)/etc/bash_completion.d/kubectl-odh

Zsh:
  $ source <(kubectl-odh completion zsh)

  # To load completions for each session, execute once:
  $ kubectl-odh completion zsh > "${fpath[1]}/_kubectl-odh"

Fish:
  $ kubectl-odh completion fish | source

  # To load completions for each session, execute once:
  $ kubectl-odh completion fish > ~/.config/fish/completions/kubectl-odh.fish
`

const bashLong = `Generate bash completion script.

Installation:
  # Linux (persistent)
  kubectl-odh completion bash > /etc/bash_completion.d/kubectl-odh

  # macOS with Homebrew
  kubectl-odh completion bash > $(brew --prefix)/etc/bash_completion.d/kubectl-odh

  # Current session only
  source <(kubectl-odh completion bash)

Note: Requires bash-completion v2 (bash 4.1+).
Run 'type _init_completion' to verify bash-completion is installed.
`

const zshLong = `Generate zsh completion script.

Installation:
  # To load completions for each session, execute once:
  kubectl-odh completion zsh > "${fpath[1]}/_kubectl-odh"

  # Current session only
  source <(kubectl-odh completion zsh)

Note: You may need to start a new shell for completions to take effect.
If completions are not working, try adding this to your ~/.zshrc:
  autoload -Uz compinit && compinit
`

const fishLong = `Generate fish completion script.

Installation:
  # To load completions for each session, execute once:
  kubectl-odh completion fish > ~/.config/fish/completions/kubectl-odh.fish

  # Current session only
  kubectl-odh completion fish | source
`

// AddCommand adds the completion command and its shell-specific subcommands.
func AddCommand(root *cobra.Command, _ *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newBashCmd(root))
	cmd.AddCommand(newZshCmd(root))
	cmd.AddCommand(newFishCmd(root))

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}

func newBashCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                   "bash",
		Short:                 "Generate bash completion script",
		Long:                  bashLong,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
		},
	}
}

func newZshCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                   "zsh",
		Short:                 "Generate zsh completion script",
		Long:                  zshLong,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenZshCompletion(cmd.OutOrStdout())
		},
	}
}

func newFishCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                   "fish",
		Short:                 "Generate fish completion script",
		Long:                  fishLong,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		},
	}
}
