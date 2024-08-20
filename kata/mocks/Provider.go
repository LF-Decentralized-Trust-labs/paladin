// Code generated by mockery v2.44.1. DO NOT EDIT.

package mocks

import (
	context "context"

	loader "github.com/kaleido-io/paladin/kata/internal/plugins/loader"
	mock "github.com/stretchr/testify/mock"

	plugins "github.com/kaleido-io/paladin/kata/internal/plugins"
)

// Provider is an autogenerated mock type for the Provider type
type Provider struct {
	mock.Mock
}

// CreateInstance provides a mock function with given fields: ctx, instanceName
func (_m *Provider) CreateInstance(ctx context.Context, instanceName string) (plugins.Instance, error) {
	ret := _m.Called(ctx, instanceName)

	if len(ret) == 0 {
		panic("no return value specified for CreateInstance")
	}

	var r0 plugins.Instance
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (plugins.Instance, error)); ok {
		return rf(ctx, instanceName)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) plugins.Instance); ok {
		r0 = rf(ctx, instanceName)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(plugins.Instance)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, instanceName)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetBinding provides a mock function with given fields:
func (_m *Provider) GetBinding() loader.Binding {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetBinding")
	}

	var r0 loader.Binding
	if rf, ok := ret.Get(0).(func() loader.Binding); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(loader.Binding)
	}

	return r0
}

// GetBuildInfo provides a mock function with given fields:
func (_m *Provider) GetBuildInfo() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetBuildInfo")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetDestination provides a mock function with given fields:
func (_m *Provider) GetDestination() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetDestination")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetName provides a mock function with given fields:
func (_m *Provider) GetName() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetName")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetType provides a mock function with given fields:
func (_m *Provider) GetType() plugins.PluginType {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetType")
	}

	var r0 plugins.PluginType
	if rf, ok := ret.Get(0).(func() plugins.PluginType); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(plugins.PluginType)
	}

	return r0
}

// NewProvider creates a new instance of Provider. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewProvider(t interface {
	mock.TestingT
	Cleanup(func())
}) *Provider {
	mock := &Provider{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
