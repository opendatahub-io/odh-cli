package status_test

import (
	"testing"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/output"
	"github.com/opendatahub-io/odh-cli/pkg/status"

	. "github.com/onsi/gomega"
)

// fixedTestTime provides a deterministic timestamp for test data.
//
//nolint:gochecknoglobals // Test data constant
var fixedTestTime = time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

func TestNewStatusReport_EnvelopeFields(t *testing.T) {
	g := NewWithT(t)

	report := &clusterhealth.Report{
		CollectedAt: fixedTestTime,
	}

	sr := status.NewStatusReport(report, nil)

	g.Expect(sr.APIVersion).To(Equal(output.APIVersion))
	g.Expect(sr.Kind).To(Equal("StatusReport"))
	g.Expect(sr.Metadata.Command).To(Equal("status"))
	g.Expect(sr.Report).To(Equal(report))
}

func TestNewStatusReport_ComputeStatus_Healthy(t *testing.T) {
	g := NewWithT(t)

	report := &clusterhealth.Report{
		CollectedAt: fixedTestTime,
		Nodes:       clusterhealth.SectionResult[clusterhealth.NodesSection]{},
		Deployments: clusterhealth.SectionResult[clusterhealth.DeploymentsSection]{},
		Pods:        clusterhealth.SectionResult[clusterhealth.PodsSection]{},
		Events:      clusterhealth.SectionResult[clusterhealth.EventsSection]{},
		Quotas:      clusterhealth.SectionResult[clusterhealth.QuotasSection]{},
		Operator:    clusterhealth.SectionResult[clusterhealth.OperatorSection]{},
		DSCI:        clusterhealth.SectionResult[clusterhealth.CRConditionsSection]{},
		DSC:         clusterhealth.SectionResult[clusterhealth.CRConditionsSection]{},
	}

	sr := status.NewStatusReport(report, nil)

	g.Expect(sr.Status).NotTo(BeNil())
	g.Expect(sr.Status.Result).To(Equal(output.StatusSuccess))
	g.Expect(sr.Status.Errors).To(Equal(0))
	g.Expect(sr.Status.Warnings).To(Equal(0))
}

func TestNewStatusReport_ComputeStatus_WithSectionErrors(t *testing.T) {
	g := NewWithT(t)

	report := &clusterhealth.Report{
		CollectedAt: fixedTestTime,
		Nodes:       clusterhealth.SectionResult[clusterhealth.NodesSection]{Error: "unhealthy nodes"},
		Deployments: clusterhealth.SectionResult[clusterhealth.DeploymentsSection]{Error: "deployments not ready"},
		Pods:        clusterhealth.SectionResult[clusterhealth.PodsSection]{},
		Events:      clusterhealth.SectionResult[clusterhealth.EventsSection]{},
		Quotas:      clusterhealth.SectionResult[clusterhealth.QuotasSection]{},
		Operator:    clusterhealth.SectionResult[clusterhealth.OperatorSection]{},
		DSCI:        clusterhealth.SectionResult[clusterhealth.CRConditionsSection]{},
		DSC:         clusterhealth.SectionResult[clusterhealth.CRConditionsSection]{},
	}

	sr := status.NewStatusReport(report, nil)

	g.Expect(sr.Status).NotTo(BeNil())
	g.Expect(sr.Status.Result).To(Equal(output.StatusFailure))
	g.Expect(sr.Status.Errors).To(Equal(2))
}

func TestNewStatusReport_ComputeStatus_WithDependencyErrors(t *testing.T) {
	g := NewWithT(t)

	report := &clusterhealth.Report{
		CollectedAt: fixedTestTime,
	}

	depStatuses := []deps.DependencyStatus{
		{Name: "servicemesh", Status: deps.StatusInstalled},
		{Name: "serverless", Status: deps.StatusMissing},
		{Name: "authorino", Status: deps.StatusUnknown},
		{Name: "certmanager", Status: deps.StatusPending},
	}

	sr := status.NewStatusReport(report, depStatuses)

	g.Expect(sr.Status).NotTo(BeNil())
	g.Expect(sr.Status.Errors).To(Equal(1), "missing dep should count as error")
	g.Expect(sr.Status.Warnings).To(Equal(2), "unknown and pending deps should count as warnings")
}

func TestNewStatusReport_ComputeStatus_WithDependencyAPIError(t *testing.T) {
	g := NewWithT(t)

	report := &clusterhealth.Report{
		CollectedAt: fixedTestTime,
	}

	depStatuses := []deps.DependencyStatus{
		{Name: "servicemesh", Status: deps.StatusInstalled},
		{Name: "serverless", Status: deps.StatusUnknown, Error: "get subscription failed"},
	}

	sr := status.NewStatusReport(report, depStatuses)

	g.Expect(sr.Status).NotTo(BeNil())
	g.Expect(sr.Status.Errors).To(Equal(1), "dep.Error should count as error")
	g.Expect(sr.Status.Warnings).To(Equal(0), "dep.Error should not count as warning")
}

func TestNewStatusReport_IncludesDependencies(t *testing.T) {
	g := NewWithT(t)

	report := &clusterhealth.Report{
		CollectedAt: fixedTestTime,
	}

	depStatuses := []deps.DependencyStatus{
		{Name: "servicemesh", DisplayName: "Service Mesh", Status: deps.StatusInstalled},
		{Name: "serverless", DisplayName: "Serverless", Status: deps.StatusMissing},
	}

	sr := status.NewStatusReport(report, depStatuses)

	g.Expect(sr.Dependencies).To(HaveLen(2))
	g.Expect(sr.Dependencies[0].Name).To(Equal("servicemesh"))
}
