package backup

import (
	"errors"
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

const DefaultTimeout = 10 * time.Minute

// SharedOptions contains options common to backup operations.
type SharedOptions struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags
	Verbose     bool
	Timeout     time.Duration
	Client      client.Client

	// Throttling settings for Kubernetes API client
	QPS   float32
	Burst int
}

// NewSharedOptions creates a new SharedOptions with defaults.
func NewSharedOptions(streams genericiooptions.IOStreams) *SharedOptions {
	return &SharedOptions{
		ConfigFlags: genericclioptions.NewConfigFlags(true),
		Timeout:     DefaultTimeout,
		IO:          iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		QPS:         client.DefaultQPS,
		Burst:       client.DefaultBurst,
	}
}

// Complete populates the client and performs pre-validation setup.
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

// Validate checks that all required options are valid.
func (o *SharedOptions) Validate() error {
	if o.Timeout <= 0 {
		return errors.New("timeout must be greater than 0")
	}

	return nil
}
