// Code generated by mockery v2.44.2. DO NOT EDIT.

package componentmocks

import (
	components "github.com/kaleido-io/paladin/kata/internal/components"
	mock "github.com/stretchr/testify/mock"
)

// Managers is an autogenerated mock type for the Managers type
type Managers struct {
	mock.Mock
}

// AllManagers provides a mock function with given fields:
func (_m *Managers) AllManagers() []components.ManagerLifecycle {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for AllManagers")
	}

	var r0 []components.ManagerLifecycle
	if rf, ok := ret.Get(0).(func() []components.ManagerLifecycle); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]components.ManagerLifecycle)
		}
	}

	return r0
}

// DomainManager provides a mock function with given fields:
func (_m *Managers) DomainManager() components.DomainManager {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DomainManager")
	}

	var r0 components.DomainManager
	if rf, ok := ret.Get(0).(func() components.DomainManager); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(components.DomainManager)
		}
	}

	return r0
}

// NewManagers creates a new instance of Managers. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewManagers(t interface {
	mock.TestingT
	Cleanup(func())
}) *Managers {
	mock := &Managers{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
