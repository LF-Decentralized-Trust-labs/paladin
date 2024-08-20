// Code generated by mockery v2.44.1. DO NOT EDIT.

package mocks

import (
	context "context"

	rpcbackend "github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	mock "github.com/stretchr/testify/mock"
)

// RPCHandler is an autogenerated mock type for the RPCHandler type
type RPCHandler struct {
	mock.Mock
}

// Execute provides a mock function with given fields: ctx, req
func (_m *RPCHandler) Execute(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
	ret := _m.Called(ctx, req)

	if len(ret) == 0 {
		panic("no return value specified for Execute")
	}

	var r0 *rpcbackend.RPCResponse
	if rf, ok := ret.Get(0).(func(context.Context, *rpcbackend.RPCRequest) *rpcbackend.RPCResponse); ok {
		r0 = rf(ctx, req)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*rpcbackend.RPCResponse)
		}
	}

	return r0
}

// NewRPCHandler creates a new instance of RPCHandler. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewRPCHandler(t interface {
	mock.TestingT
	Cleanup(func())
}) *RPCHandler {
	mock := &RPCHandler{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
