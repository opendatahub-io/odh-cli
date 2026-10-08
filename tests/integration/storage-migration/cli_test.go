//go:build integration

package storagemigration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/cmd/lint"
	"github.com/opendatahub-io/odh-cli/cmd/migrate"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"

	. "github.com/onsi/gomega"
)

const (
	targetRelease    = "3.6.0"
	checkID          = "platform.dsc.storage-version"
	migrationID      = "dsc.storage-version.migrate"
	migrationRequest = "dsc-storage-migration"
	migrationTimeout = "90s"
)

// cli runs the odh commands in-process against a cluster.
type cli struct {
	cluster *cluster
}

func newCLI(c *cluster) *cli {
	return &cli{
		cluster: c,
	}
}

// migrate runs the storage version migration and expects it to succeed.
func (o *cli) migrate(t *testing.T, extraArgs ...string) {
	t.Helper()

	err := o.runMigration(t, extraArgs...)
	if err != nil {
		o.logMigrationRequest(t)
	}

	NewWithT(t).Expect(err).ToNot(HaveOccurred())
}

// migrateExpectingError runs the storage version migration, expects it to
// fail, and returns the error for the caller to inspect.
func (o *cli) migrateExpectingError(t *testing.T, extraArgs ...string) error {
	t.Helper()

	err := o.runMigration(t, extraArgs...)

	NewWithT(t).Expect(err).To(HaveOccurred())

	return err
}

func (o *cli) runMigration(t *testing.T, extraArgs ...string) error {
	t.Helper()

	args := []string{
		"migrate", "run",
		"--migration", migrationID,
		"--target-version", targetRelease,
		"--timeout", migrationTimeout,
		"--output", "json",
		"--yes",
	}

	_, err := o.runCLI(t, append(args, extraArgs...)...)

	return err
}

func (o *cli) expectLintBlocks(t *testing.T) {
	t.Helper()
	g := NewWithT(t)

	finding, err := o.runLint(t)

	g.Expect(err).To(HaveOccurred())
	g.Expect(finding.GetImpact()).To(Equal(result.ImpactBlocking))
}

func (o *cli) expectLintPasses(t *testing.T) {
	t.Helper()
	g := NewWithT(t)

	finding, err := o.runLint(t)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(finding.IsFailing()).To(BeFalse())
}

// runLint runs only the storage version check and returns its single result,
// along with the command error (set when the check blocks).
func (o *cli) runLint(t *testing.T) (*result.DiagnosticResult, error) {
	t.Helper()
	g := NewWithT(t)

	output, err := o.runCLI(t,
		"lint",
		"--checks", checkID,
		"--target-version", targetRelease,
		"--output", "json",
	)

	var diagnostics result.DiagnosticResultList
	g.Expect(json.Unmarshal(output, &diagnostics)).To(Succeed())
	g.Expect(diagnostics.Results).To(HaveLen(1))

	return diagnostics.Results[0], err
}

// runCLI runs the odh commands in-process, wired the same way as cmd/main.go,
// against the test cluster. Every call builds a new command tree so flag values
// do not leak between invocations.
func (o *cli) runCLI(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	root := &cobra.Command{
		Use:           "kubectl-odh",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// The subcommands capture the root streams when they are added.
	root.SetIn(strings.NewReader(""))
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	flags := genericclioptions.NewConfigFlags(true)
	flags.AddFlags(root.PersistentFlags())

	lint.AddCommand(root, flags)
	migrate.AddCommand(root, flags)

	// An external cluster has no kubeconfig of its own: the commands then
	// resolve the ambient one, exactly as they do for a user.
	if o.cluster.kubeconfig != "" {
		args = append([]string{"--kubeconfig", o.cluster.kubeconfig}, args...)
	}

	root.SetArgs(args)

	err := root.ExecuteContext(t.Context())

	t.Logf("odh %s\n%s", strings.Join(args, " "), stderr.String())

	// Handled errors are only printed to stderr, so carry that text along.
	if err != nil {
		err = fmt.Errorf("running CLI: %w: %s", err, stderr.String())
	}

	return stdout.Bytes(), err
}

// logMigrationRequest records what the controller reported, to explain a failed run.
func (o *cli) logMigrationRequest(t *testing.T) {
	t.Helper()

	api := o.cluster.migrationAPI(t)
	if api == nil {
		return
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	request, err := o.cluster.dynamic.Resource(api.GVR()).Get(ctx, migrationRequest, metav1.GetOptions{})

	switch {
	case err != nil:
		t.Logf("Reading migration request after command error: %v", err)
	default:
		t.Logf("Migration request status after command error: %#v", request.Object["status"])
	}
}
