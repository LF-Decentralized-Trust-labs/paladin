// Code generated by mockery v2.44.1. DO NOT EDIT.

package mocks

import (
	context "context"

	transactionstore "github.com/kaleido-io/paladin/kata/internal/transactionstore"
	mock "github.com/stretchr/testify/mock"
)

// TxStateManager is an autogenerated mock type for the TxStateManager type
type TxStateManager struct {
	mock.Mock
}

// ApplyTxUpdates provides a mock function with given fields: ctx, txUpdates
func (_m *TxStateManager) ApplyTxUpdates(ctx context.Context, txUpdates *transactionstore.TransactionUpdate) {
	_m.Called(ctx, txUpdates)
}

// GetConfirmedTxHash provides a mock function with given fields: ctx
func (_m *TxStateManager) GetConfirmedTxHash(ctx context.Context) string {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetConfirmedTxHash")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetContract provides a mock function with given fields: ctx
func (_m *TxStateManager) GetContract(ctx context.Context) string {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetContract")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetDispatchAddress provides a mock function with given fields: ctx
func (_m *TxStateManager) GetDispatchAddress(ctx context.Context) string {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetDispatchAddress")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetDispatchNode provides a mock function with given fields: ctx
func (_m *TxStateManager) GetDispatchNode(ctx context.Context) string {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetDispatchNode")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetDispatchTxID provides a mock function with given fields: ctx
func (_m *TxStateManager) GetDispatchTxID(ctx context.Context) string {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetDispatchTxID")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetDispatchTxPayload provides a mock function with given fields: ctx
func (_m *TxStateManager) GetDispatchTxPayload(ctx context.Context) string {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetDispatchTxPayload")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetPreReqTransactions provides a mock function with given fields: ctx
func (_m *TxStateManager) GetPreReqTransactions(ctx context.Context) []string {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetPreReqTransactions")
	}

	var r0 []string
	if rf, ok := ret.Get(0).(func(context.Context) []string); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	return r0
}

// GetTxID provides a mock function with given fields: ctx
func (_m *TxStateManager) GetTxID(ctx context.Context) string {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetTxID")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// NewTxStateManager creates a new instance of TxStateManager. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewTxStateManager(t interface {
	mock.TestingT
	Cleanup(func())
}) *TxStateManager {
	mock := &TxStateManager{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
