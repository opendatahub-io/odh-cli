package check_test

import (
	"context"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"

	. "github.com/onsi/gomega"
)

// Test 1: Valid Context (no timeout, not canceled).
func TestCheckContextError_ValidContext(t *testing.T) {
	g := NewWithT(t)

	ctx := context.Background()
	err := check.CheckContextError(ctx)
	g.Expect(err).ToNot(HaveOccurred())
}

// Test 2: Timeout Exceeded.
func TestCheckContextError_TimeoutExceeded(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Wait for timeout to trigger
	time.Sleep(10 * time.Millisecond)

	err := check.CheckContextError(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(Equal(check.ErrCheckTimeout))
}

// Test 3: Context Canceled.
func TestCheckContextError_Canceled(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	err := check.CheckContextError(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(Equal(check.ErrCheckCanceled))
}

// Test 4: Context with deadline in future (not yet exceeded).
func TestCheckContextError_DeadlineNotYetExceeded(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	err := check.CheckContextError(ctx)
	g.Expect(err).ToNot(HaveOccurred())
}
