//nolint:testpackage // The small injected limit tests the unexported reader wrapper without a 512 MiB fixture.
package rhbok

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

const (
	testCatalogExpectedChannel = "stable-v1.3"
	testCatalogLimitError      = "content exceeds"
	testCatalogWithinLimit     = `{"schema":"olm.package","name":"kueue-operator","defaultChannel":"stable-v1.2"}
{"schema":"olm.channel","package":"kueue-operator","name":"stable-v1.2"}
{"schema":"olm.channel","package":"kueue-operator","name":"stable-v1.3"}
`
	testCatalogBeyondLimit = `{"schema":"olm.channel","package":"kueue-operator","name":"stable-v9.9"}
`
	testCatalogLimit          = int64(len(testCatalogWithinLimit))
	testTruncatedCatalogLimit = testCatalogLimit - 5
)

func TestResolveRHBOKCatalogContentChannelLimit(t *testing.T) {
	t.Run("scans every entry at the limit", func(t *testing.T) {
		g := NewWithT(t)
		channel, err := resolveRHBOKCatalogContentChannel(strings.NewReader(testCatalogWithinLimit), testCatalogLimit)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(channel).To(Equal(testCatalogExpectedChannel))
	})

	t.Run("rejects a complete catalog beyond the limit", func(t *testing.T) {
		g := NewWithT(t)
		content := strings.NewReader(testCatalogWithinLimit + testCatalogBeyondLimit)
		channel, err := resolveRHBOKCatalogContentChannel(content, testCatalogLimit)
		g.Expect(channel).To(BeEmpty())
		g.Expect(err).To(MatchError(ContainSubstring(testCatalogLimitError)))
	})

	t.Run("reports the limit when an entry is truncated", func(t *testing.T) {
		g := NewWithT(t)
		channel, err := resolveRHBOKCatalogContentChannel(strings.NewReader(testCatalogWithinLimit), testTruncatedCatalogLimit)
		g.Expect(channel).To(BeEmpty())
		g.Expect(err).To(MatchError(ContainSubstring(testCatalogLimitError)))
	})
}
