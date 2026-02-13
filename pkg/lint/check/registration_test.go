package check_test

import (
	"fmt"
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	mocks "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/check"

	. "github.com/onsi/gomega"
)

// Test 1: Duplicate Registration Error.
func TestRegistry_Register_DuplicateCheckPanic(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry() // Isolated registry for testing

	// Create mock check with specific ID
	mockCheck := mocks.NewMockCheck()
	mockCheck.On("ID").Return("test.duplicate")
	mockCheck.On("Name").Return("Test Duplicate")
	mockCheck.On("Group").Return(check.GroupComponent)

	// First registration should succeed
	err := registry.Register(mockCheck)
	g.Expect(err).ToNot(HaveOccurred())

	// Second registration with same ID should fail
	err = registry.Register(mockCheck)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("already registered"))
	g.Expect(err.Error()).To(ContainSubstring("test.duplicate"))
}

// Test 2: Successful Registration.
func TestRegistry_Register_Success(t *testing.T) {
	g := NewWithT(t)

	// Create new registry for isolation
	registry := check.NewRegistry()

	// Create mock check
	mockCheck := mocks.NewMockCheck()
	mockCheck.On("ID").Return("test.success")
	mockCheck.On("Name").Return("Test Success")
	mockCheck.On("Group").Return(check.GroupComponent)

	// Register via registry method
	err := registry.Register(mockCheck)
	g.Expect(err).ToNot(HaveOccurred())

	// Verify registration
	retrieved, exists := registry.Get("test.success")
	g.Expect(exists).To(BeTrue())
	g.Expect(retrieved.ID()).To(Equal("test.success"))
}

// Test 3: MustRegisterCheck Panic on Duplicate.
func TestMustRegisterCheck_PanicOnDuplicate(t *testing.T) {
	g := NewWithT(t)

	// Create isolated registry for testing
	registry := check.NewRegistry()

	// Create first mock check
	mockCheck1 := mocks.NewMockCheck()
	mockCheck1.On("ID").Return("test.panic")
	mockCheck1.On("Name").Return("Test Panic 1")
	mockCheck1.On("Group").Return(check.GroupComponent)

	// Register first check
	err := registry.Register(mockCheck1)
	g.Expect(err).ToNot(HaveOccurred())

	// Create second mock with same ID
	mockCheck2 := mocks.NewMockCheck()
	mockCheck2.On("ID").Return("test.panic")
	mockCheck2.On("Name").Return("Test Panic 2")
	mockCheck2.On("Group").Return(check.GroupComponent)

	// Attempt to register duplicate should return error
	err = registry.Register(mockCheck2)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("already registered"))
	g.Expect(err.Error()).To(ContainSubstring("test.panic"))
}

// Test 4: Concurrent Registration Safety.
func TestRegistry_ConcurrentRegistration(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	// Launch 10 goroutines trying to register checks concurrently
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := range numGoroutines {
		go func(index int) {
			defer func() { done <- true }()

			mockCheck := mocks.NewMockCheck()
			checkID := fmt.Sprintf("concurrent.test.%d", index)
			mockCheck.On("ID").Return(checkID)
			mockCheck.On("Name").Return(fmt.Sprintf("Concurrent Test %d", index))
			mockCheck.On("Group").Return(check.GroupComponent)

			err := registry.Register(mockCheck)
			if err != nil {
				t.Errorf("Failed to register check %s: %v", checkID, err)
			}
		}(i)
	}

	// Wait for all goroutines
	for range numGoroutines {
		<-done
	}

	// Verify all checks registered
	allChecks := registry.ListAll()
	g.Expect(allChecks).To(HaveLen(numGoroutines))
}

// Test 5: Global Registry Expected Check Count.
// Test 5 & 6: Global Registry tests REMOVED.
// Global registry eliminated in favor of explicit dependency injection.
// Checks are now registered directly in NewCommand().

// Test 7: ListByGroup filtering.
func TestRegistry_ListByGroup(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	// Register checks in different groups
	componentCheck := mocks.NewMockCheck()
	componentCheck.On("ID").Return("component.test")
	componentCheck.On("Group").Return(check.GroupComponent)

	dependencyCheck := mocks.NewMockCheck()
	dependencyCheck.On("ID").Return("dependency.test")
	dependencyCheck.On("Group").Return(check.GroupDependency)

	workloadCheck := mocks.NewMockCheck()
	workloadCheck.On("ID").Return("workload.test")
	workloadCheck.On("Group").Return(check.GroupWorkload)

	err := registry.Register(componentCheck)
	g.Expect(err).ToNot(HaveOccurred())
	err = registry.Register(dependencyCheck)
	g.Expect(err).ToNot(HaveOccurred())
	err = registry.Register(workloadCheck)
	g.Expect(err).ToNot(HaveOccurred())

	// Test GroupComponent filter
	components := registry.ListByGroup(check.GroupComponent)
	g.Expect(components).To(HaveLen(1))
	g.Expect(components[0].ID()).To(Equal("component.test"))

	// Test GroupDependency filter
	dependencies := registry.ListByGroup(check.GroupDependency)
	g.Expect(dependencies).To(HaveLen(1))
	g.Expect(dependencies[0].ID()).To(Equal("dependency.test"))

	// Test GroupWorkload filter
	workloads := registry.ListByGroup(check.GroupWorkload)
	g.Expect(workloads).To(HaveLen(1))
	g.Expect(workloads[0].ID()).To(Equal("workload.test"))
}

// Test 8: ListBySelector filtering.
func TestRegistry_ListBySelector(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	// Register checks in different groups
	componentCheck := mocks.NewMockCheck()
	componentCheck.On("ID").Return("component.test")
	componentCheck.On("Group").Return(check.GroupComponent)

	dependencyCheck := mocks.NewMockCheck()
	dependencyCheck.On("ID").Return("dependency.test")
	dependencyCheck.On("Group").Return(check.GroupDependency)

	err := registry.Register(componentCheck)
	g.Expect(err).ToNot(HaveOccurred())
	err = registry.Register(dependencyCheck)
	g.Expect(err).ToNot(HaveOccurred())

	// Test with empty group (returns all)
	all := registry.ListBySelector("")
	g.Expect(all).To(HaveLen(2))

	// Test with specific group
	components := registry.ListBySelector(check.GroupComponent)
	g.Expect(components).To(HaveLen(1))
	g.Expect(components[0].ID()).To(Equal("component.test"))
}
