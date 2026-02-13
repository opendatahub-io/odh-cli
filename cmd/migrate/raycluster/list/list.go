package list

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/migrate/raycluster"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

const (
	cmdName  = "list"
	cmdShort = "List RayClusters and migration status"
)

const cmdLong = `
List all RayClusters and show whether each needs migration to RHOAI 3.x.

Use --namespace to limit to a single namespace. Use -o json or -o yaml for scriptable output.
`

const cmdExample = `
  kubectl odh migrate raycluster list
  kubectl odh migrate raycluster list --namespace my-ns
  kubectl odh migrate raycluster list -o json
`

type options struct {
	ConfigFlags  *genericclioptions.ConfigFlags
	IO           iostreams.Interface
	Client       client.Client
	OutputFormat string
	Namespace    string
}

func (o *options) Complete() error {
	restConfig, err := client.NewRESTConfig(o.ConfigFlags, client.DefaultQPS, client.DefaultBurst)
	if err != nil {
		return fmt.Errorf("create REST config: %w", err)
	}
	c, err := client.NewClientWithConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}
	o.Client = c
	if o.Namespace == "" && o.ConfigFlags.Namespace != nil && *o.ConfigFlags.Namespace != "" {
		o.Namespace = *o.ConfigFlags.Namespace
	}

	return nil
}

func (o *options) Validate() error {
	switch o.OutputFormat {
	case "table", "json", "yaml":
		return nil
	default:
		return fmt.Errorf("invalid output format: %s (must be table, json, or yaml)", o.OutputFormat)
	}
}

func (o *options) Run(ctx context.Context) error {
	_, err := raycluster.ListRayClusters(ctx, o.Client, o.Namespace, o.OutputFormat, o.IO)
	return err
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.OutputFormat, "output", "o", "table", "Output format (table|json|yaml)")
	fs.StringVarP(&o.Namespace, "namespace", "n", "", "List clusters in this namespace only")
}

// AddCommand adds the list subcommand to the raycluster command.
func AddCommand(
	parent *cobra.Command,
	flags *genericclioptions.ConfigFlags,
	streams genericiooptions.IOStreams,
) {
	o := &options{
		ConfigFlags:  flags,
		IO:           iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		OutputFormat: "table",
	}

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run(cmd.Context())
		},
	}

	o.AddFlags(cmd.Flags())
	parent.AddCommand(cmd)
}
