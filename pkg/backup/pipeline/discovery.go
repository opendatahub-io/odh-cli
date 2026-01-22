package pipeline

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

// DiscoveryStage lists workload instances and sends them to output channel.
type DiscoveryStage struct {
	Client  *client.Client
	Verbose bool
	IO      iostreams.Interface
}

// Run executes discovery for a workload type.
func (d *DiscoveryStage) Run(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	output chan<- WorkloadItem,
) error {
	instances, err := d.Client.ListResources(ctx, gvr)
	if err != nil {
		return fmt.Errorf("listing resources: %w", err)
	}

	if d.Verbose {
		d.IO.Errorf("Found %d %s\n", len(instances), gvr.Resource)
	}

	for i := range instances {
		select {
		case <-ctx.Done():
			return fmt.Errorf("discovery cancelled: %w", ctx.Err())
		case output <- WorkloadItem{GVR: gvr, Instance: instances[i]}:
		}
	}

	return nil
}
