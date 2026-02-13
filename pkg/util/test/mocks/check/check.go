package mocks

import (
	"context"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/stretchr/testify/mock"
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

func (m *MockCheck) Group() check.CheckGroup {
	args := m.Called()
	group, ok := args.Get(0).(check.CheckGroup)
	if !ok {
		return check.GroupComponent
	}

	return group
}

func (m *MockCheck) CheckKind() string {
	args := m.Called()

	return args.String(0)
}

func (m *MockCheck) CheckType() string {
	args := m.Called()

	return args.String(0)
}

func (m *MockCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	args := m.Called(ctx, target)

	return args.Bool(0), args.Error(1)
}

func (m *MockCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	args := m.Called(ctx, target)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	dr, ok := args.Get(0).(*result.DiagnosticResult)
	if !ok {
		return nil, args.Error(1)
	}

	return dr, args.Error(1)
}
