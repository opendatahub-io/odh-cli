package client_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"

	. "github.com/onsi/gomega"
)

func TestConfigureThrottling(t *testing.T) {
	g := NewWithT(t)

	t.Run("should configure QPS and Burst on REST config", func(t *testing.T) {
		config := &rest.Config{}

		client.ConfigureThrottling(config, 50, 100)

		g.Expect(config.QPS).To(Equal(float32(50)))
		g.Expect(config.Burst).To(Equal(100))
	})

	t.Run("should allow custom values", func(t *testing.T) {
		config := &rest.Config{}

		client.ConfigureThrottling(config, 75.5, 150)

		g.Expect(config.QPS).To(Equal(float32(75.5)))
		g.Expect(config.Burst).To(Equal(150))
	})

	t.Run("should override existing values", func(t *testing.T) {
		config := &rest.Config{
			QPS:   10,
			Burst: 20,
		}

		client.ConfigureThrottling(config, 50, 100)

		g.Expect(config.QPS).To(Equal(float32(50)))
		g.Expect(config.Burst).To(Equal(100))
	})
}

func TestNewRESTConfig(t *testing.T) {
	g := NewWithT(t)

	t.Run("should create REST config with specified throttling", func(t *testing.T) {
		configFlags := genericclioptions.NewConfigFlags(true)

		config, err := client.NewRESTConfig(configFlags, 75, 150)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(config).ToNot(BeNil())
		g.Expect(config.QPS).To(Equal(float32(75)))
		g.Expect(config.Burst).To(Equal(150))
	})

	t.Run("should use default throttling values", func(t *testing.T) {
		configFlags := genericclioptions.NewConfigFlags(true)

		config, err := client.NewRESTConfig(configFlags, client.DefaultQPS, client.DefaultBurst)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(config).ToNot(BeNil())
		g.Expect(config.QPS).To(BeEquivalentTo(client.DefaultQPS))
		g.Expect(config.Burst).To(Equal(client.DefaultBurst))
	})
}

func TestDefaultConstants(t *testing.T) {
	g := NewWithT(t)

	t.Run("should have appropriate default values", func(t *testing.T) {
		// DefaultQPS should be higher than kubectl's default (5)
		g.Expect(client.DefaultQPS).To(BeNumerically(">", 5))

		// DefaultBurst should be higher than kubectl's default (10)
		g.Expect(client.DefaultBurst).To(BeNumerically(">", 10))

		// Verify specific expected values from plan
		g.Expect(client.DefaultQPS).To(BeEquivalentTo(50))
		g.Expect(client.DefaultBurst).To(Equal(100))
	})
}
