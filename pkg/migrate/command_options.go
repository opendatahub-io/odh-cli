package migrate

import (
	"errors"
	"fmt"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table"
	OutputFormatJSON  OutputFormat = "json"
	OutputFormatYAML  OutputFormat = "yaml"

	DefaultTimeout = 10 * time.Minute
)

func (o OutputFormat) Validate() error {
	switch o {
	case OutputFormatTable, OutputFormatJSON, OutputFormatYAML:
		return nil
	default:
		return fmt.Errorf("invalid output format: %s (must be one of: table, json, yaml)", o)
	}
}

type SharedOptions struct {
	IO           iostreams.Interface
	ConfigFlags  *genericclioptions.ConfigFlags
	OutputFormat OutputFormat
	Verbose      bool
	Timeout      time.Duration
	Client       client.Client

	// Throttling settings for Kubernetes API client
	QPS   float32
	Burst int
}

func NewSharedOptions(streams genericiooptions.IOStreams) *SharedOptions {
	return &SharedOptions{
		ConfigFlags:  genericclioptions.NewConfigFlags(true),
		OutputFormat: OutputFormatTable,
		Timeout:      DefaultTimeout,
		IO:           iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		QPS:          client.DefaultQPS,
		Burst:        client.DefaultBurst,
	}
}

func (o *SharedOptions) Complete() error {
	// Create REST config with user-specified throttling
	restConfig, err := client.NewRESTConfig(o.ConfigFlags, o.QPS, o.Burst)
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	// Create client with configured throttling
	c, err := client.NewClientWithConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	o.Client = c

	return nil
}

func (o *SharedOptions) Validate() error {
	if err := o.OutputFormat.Validate(); err != nil {
		return err
	}

	if o.Timeout <= 0 {
		return errors.New("timeout must be greater than 0")
	}

	return nil
}
