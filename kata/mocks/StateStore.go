// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	context "context"

	statestore "github.com/kaleido-io/paladin/kata/internal/statestore"
	mock "github.com/stretchr/testify/mock"
)

// StateStore is an autogenerated mock type for the StateStore type
type StateStore struct {
	mock.Mock
}

// Close provides a mock function with given fields:
func (_m *StateStore) Close() {
	_m.Called()
}

// GetSchema provides a mock function with given fields: _a0, _a1, _a2
func (_m *StateStore) GetSchema(_a0 context.Context, _a1 string, _a2 *statestore.HashID) (statestore.Schema, error) {
	ret := _m.Called(_a0, _a1, _a2)

	if len(ret) == 0 {
		panic("no return value specified for GetSchema")
	}

	var r0 statestore.Schema
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, *statestore.HashID) (statestore.Schema, error)); ok {
		return rf(_a0, _a1, _a2)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, *statestore.HashID) statestore.Schema); ok {
		r0 = rf(_a0, _a1, _a2)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(statestore.Schema)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, *statestore.HashID) error); ok {
		r1 = rf(_a0, _a1, _a2)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetState provides a mock function with given fields: ctx, domainID, hash, withLabels
func (_m *StateStore) GetState(ctx context.Context, domainID string, hash *statestore.HashID, withLabels bool) (*statestore.State, error) {
	ret := _m.Called(ctx, domainID, hash, withLabels)

	if len(ret) == 0 {
		panic("no return value specified for GetState")
	}

	var r0 *statestore.State
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, *statestore.HashID, bool) (*statestore.State, error)); ok {
		return rf(ctx, domainID, hash, withLabels)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, *statestore.HashID, bool) *statestore.State); ok {
		r0 = rf(ctx, domainID, hash, withLabels)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*statestore.State)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, *statestore.HashID, bool) error); ok {
		r1 = rf(ctx, domainID, hash, withLabels)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewStateStore creates a new instance of StateStore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewStateStore(t interface {
	mock.TestingT
	Cleanup(func())
}) *StateStore {
	mock := &StateStore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
