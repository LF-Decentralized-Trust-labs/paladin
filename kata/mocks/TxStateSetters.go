// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	context "context"

	transactionstore "github.com/kaleido-io/paladin/kata/internal/transactionstore"
	mock "github.com/stretchr/testify/mock"
)

// TxStateSetters is an autogenerated mock type for the TxStateSetters type
type TxStateSetters struct {
	mock.Mock
}

// ApplyTxUpdates provides a mock function with given fields: ctx, txUpdates
func (_m *TxStateSetters) ApplyTxUpdates(ctx context.Context, txUpdates *transactionstore.TransactionUpdate) {
	_m.Called(ctx, txUpdates)
}

// NewTxStateSetters creates a new instance of TxStateSetters. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewTxStateSetters(t interface {
	mock.TestingT
	Cleanup(func())
}) *TxStateSetters {
	mock := &TxStateSetters{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
