// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	abi "github.com/hyperledger/firefly-signer/pkg/abi"
	filters "github.com/kaleido-io/paladin/kata/internal/filters"

	mock "github.com/stretchr/testify/mock"

	statestore "github.com/kaleido-io/paladin/kata/internal/statestore"

	types "github.com/kaleido-io/paladin/kata/internal/types"

	uuid "github.com/google/uuid"
)

// DomainStateInterface is an autogenerated mock type for the DomainStateInterface type
type DomainStateInterface struct {
	mock.Mock
}

// EnsureABISchemas provides a mock function with given fields: defs
func (_m *DomainStateInterface) EnsureABISchemas(defs []*abi.Parameter) ([]*statestore.Schema, error) {
	ret := _m.Called(defs)

	if len(ret) == 0 {
		panic("no return value specified for EnsureABISchemas")
	}

	var r0 []*statestore.Schema
	var r1 error
	if rf, ok := ret.Get(0).(func([]*abi.Parameter) ([]*statestore.Schema, error)); ok {
		return rf(defs)
	}
	if rf, ok := ret.Get(0).(func([]*abi.Parameter) []*statestore.Schema); ok {
		r0 = rf(defs)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*statestore.Schema)
		}
	}

	if rf, ok := ret.Get(1).(func([]*abi.Parameter) error); ok {
		r1 = rf(defs)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FindAvailableStates provides a mock function with given fields: schemaID, query
func (_m *DomainStateInterface) FindAvailableStates(schemaID string, query *filters.QueryJSON) ([]*statestore.State, error) {
	ret := _m.Called(schemaID, query)

	if len(ret) == 0 {
		panic("no return value specified for FindAvailableStates")
	}

	var r0 []*statestore.State
	var r1 error
	if rf, ok := ret.Get(0).(func(string, *filters.QueryJSON) ([]*statestore.State, error)); ok {
		return rf(schemaID, query)
	}
	if rf, ok := ret.Get(0).(func(string, *filters.QueryJSON) []*statestore.State); ok {
		r0 = rf(schemaID, query)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*statestore.State)
		}
	}

	if rf, ok := ret.Get(1).(func(string, *filters.QueryJSON) error); ok {
		r1 = rf(schemaID, query)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Flush provides a mock function with given fields: successCallback
func (_m *DomainStateInterface) Flush(successCallback ...statestore.DomainContextFunction) error {
	_va := make([]interface{}, len(successCallback))
	for _i := range successCallback {
		_va[_i] = successCallback[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for Flush")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(...statestore.DomainContextFunction) error); ok {
		r0 = rf(successCallback...)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MarkStatesRead provides a mock function with given fields: sequenceID, schemaID, stateIDs
func (_m *DomainStateInterface) MarkStatesRead(sequenceID uuid.UUID, schemaID string, stateIDs []string) error {
	ret := _m.Called(sequenceID, schemaID, stateIDs)

	if len(ret) == 0 {
		panic("no return value specified for MarkStatesRead")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(uuid.UUID, string, []string) error); ok {
		r0 = rf(sequenceID, schemaID, stateIDs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MarkStatesSpending provides a mock function with given fields: sequenceID, schemaID, stateIDs
func (_m *DomainStateInterface) MarkStatesSpending(sequenceID uuid.UUID, schemaID string, stateIDs []string) error {
	ret := _m.Called(sequenceID, schemaID, stateIDs)

	if len(ret) == 0 {
		panic("no return value specified for MarkStatesSpending")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(uuid.UUID, string, []string) error); ok {
		r0 = rf(sequenceID, schemaID, stateIDs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ResetSequence provides a mock function with given fields: sequenceID
func (_m *DomainStateInterface) ResetSequence(sequenceID uuid.UUID) error {
	ret := _m.Called(sequenceID)

	if len(ret) == 0 {
		panic("no return value specified for ResetSequence")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(uuid.UUID) error); ok {
		r0 = rf(sequenceID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UnitTestFlushSync provides a mock function with given fields:
func (_m *DomainStateInterface) UnitTestFlushSync() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for UnitTestFlushSync")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// WriteNewStates provides a mock function with given fields: sequenceID, schemaID, data
func (_m *DomainStateInterface) WriteNewStates(sequenceID uuid.UUID, schemaID string, data []types.RawJSON) ([]*statestore.State, error) {
	ret := _m.Called(sequenceID, schemaID, data)

	if len(ret) == 0 {
		panic("no return value specified for WriteNewStates")
	}

	var r0 []*statestore.State
	var r1 error
	if rf, ok := ret.Get(0).(func(uuid.UUID, string, []types.RawJSON) ([]*statestore.State, error)); ok {
		return rf(sequenceID, schemaID, data)
	}
	if rf, ok := ret.Get(0).(func(uuid.UUID, string, []types.RawJSON) []*statestore.State); ok {
		r0 = rf(sequenceID, schemaID, data)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*statestore.State)
		}
	}

	if rf, ok := ret.Get(1).(func(uuid.UUID, string, []types.RawJSON) error); ok {
		r1 = rf(sequenceID, schemaID, data)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewDomainStateInterface creates a new instance of DomainStateInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewDomainStateInterface(t interface {
	mock.TestingT
	Cleanup(func())
}) *DomainStateInterface {
	mock := &DomainStateInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
