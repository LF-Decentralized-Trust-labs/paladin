// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	context "context"

	statestore "github.com/kaleido-io/paladin/kata/internal/statestore"
	mock "github.com/stretchr/testify/mock"
)

// Schema is an autogenerated mock type for the Schema type
type Schema struct {
	mock.Mock
}

// Persisted provides a mock function with given fields:
func (_m *Schema) Persisted() *statestore.SchemaEntity {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Persisted")
	}

	var r0 *statestore.SchemaEntity
	if rf, ok := ret.Get(0).(func() *statestore.SchemaEntity); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*statestore.SchemaEntity)
		}
	}

	return r0
}

// ProcessState provides a mock function with given fields: ctx, s
func (_m *Schema) ProcessState(ctx context.Context, s *statestore.State) error {
	ret := _m.Called(ctx, s)

	if len(ret) == 0 {
		panic("no return value specified for ProcessState")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *statestore.State) error); ok {
		r0 = rf(ctx, s)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Type provides a mock function with given fields:
func (_m *Schema) Type() statestore.SchemaType {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Type")
	}

	var r0 statestore.SchemaType
	if rf, ok := ret.Get(0).(func() statestore.SchemaType); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(statestore.SchemaType)
	}

	return r0
}

// NewSchema creates a new instance of Schema. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewSchema(t interface {
	mock.TestingT
	Cleanup(func())
}) *Schema {
	mock := &Schema{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
