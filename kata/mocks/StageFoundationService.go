// Code generated by mockery v2.44.1. DO NOT EDIT.

package mocks

import (
	types "github.com/kaleido-io/paladin/kata/internal/engine/types"
	statestore "github.com/kaleido-io/paladin/kata/internal/statestore"
	mock "github.com/stretchr/testify/mock"
)

// StageFoundationService is an autogenerated mock type for the StageFoundationService type
type StageFoundationService struct {
	mock.Mock
}

// DependencyChecker provides a mock function with given fields:
func (_m *StageFoundationService) DependencyChecker() types.DependencyChecker {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DependencyChecker")
	}

	var r0 types.DependencyChecker
	if rf, ok := ret.Get(0).(func() types.DependencyChecker); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(types.DependencyChecker)
		}
	}

	return r0
}

// IdentityResolver provides a mock function with given fields:
func (_m *StageFoundationService) IdentityResolver() types.IdentityResolver {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for IdentityResolver")
	}

	var r0 types.IdentityResolver
	if rf, ok := ret.Get(0).(func() types.IdentityResolver); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(types.IdentityResolver)
		}
	}

	return r0
}

// StateStore provides a mock function with given fields:
func (_m *StageFoundationService) StateStore() statestore.StateStore {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for StateStore")
	}

	var r0 statestore.StateStore
	if rf, ok := ret.Get(0).(func() statestore.StateStore); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(statestore.StateStore)
		}
	}

	return r0
}

// NewStageFoundationService creates a new instance of StageFoundationService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewStageFoundationService(t interface {
	mock.TestingT
	Cleanup(func())
}) *StageFoundationService {
	mock := &StageFoundationService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
