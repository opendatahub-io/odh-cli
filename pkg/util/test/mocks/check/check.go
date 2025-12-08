package mocks

import (
	"context"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/mock"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
)

// MockCheck is a mock implementation of check.Check interface using testify/mock.
type MockCheck struct {
	mock.Mock
}

// NewMockCheck creates a new MockCheck instance.
func NewMockCheck() *MockCheck {
	return &MockCheck{}
}

func (m *MockCheck) ID() string {
	args := m.Called()

	return args.String(0)
}

func (m *MockCheck) Name() string {
	args := m.Called()

	return args.String(0)
}

func (m *MockCheck) Description() string {
	args := m.Called()

	return args.String(0)
}

func (m *MockCheck) Category() check.CheckCategory {
	args := m.Called()
	category, ok := args.Get(0).(check.CheckCategory)
	if !ok {
		return check.CategoryComponent
	}

	return category
}

func (m *MockCheck) CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool {
	args := m.Called(currentVersion, targetVersion)

	return args.Bool(0)
}

func (m *MockCheck) Validate(ctx context.Context, target *check.CheckTarget) (*check.DiagnosticResult, error) {
	args := m.Called(ctx, target)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, ok := args.Get(0).(*check.DiagnosticResult)
	if !ok {
		return nil, args.Error(1)
	}

	return result, args.Error(1)
}
