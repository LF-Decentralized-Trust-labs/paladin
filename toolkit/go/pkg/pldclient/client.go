/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package pldclient

import (
	"context"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tkmsgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type PaladinClient interface {
	// Direct RPC access
	rpcclient.Client

	// Config
	ReceiptPollingInterval(t time.Duration) PaladinClient
	HTTP(ctx context.Context, conf *pldconf.HTTPClientConfig) (PaladinClient, error)
	WebSocket(ctx context.Context, conf *pldconf.WSClientConfig) (PaladinClient, error)

	// ABI based helpers for building data payloads, and submitting transactions
	ABI(ctx context.Context, a abi.ABI) (ABIClient, error)
	MustABI(a abi.ABI) ABIClient
	ABIJSON(ctx context.Context, abiJson []byte) (ABIClient, error)
	ABIFunction(ctx context.Context, functionABI *abi.Entry) (_ ABIFunctionClient, err error)
	ABIConstructor(ctx context.Context, constructorABI *abi.Entry, bytecode tktypes.HexBytes) (_ ABIFunctionClient, err error)
	MustABIJSON(abiJson []byte) ABIClient

	// Paladin transaction RPC interface
	PTX() PTX

	// Paladin Key Manager RPC interface
	KeyManager() KeyManager
}

type PaladinWSClient interface {
	PaladinClient // No differences... yet
}

type paladinClient struct {
	rpcclient.Client
	receiptPollingInterval time.Duration
}

const (
	DefaultReceiptPollingInterval = 1 * time.Second
)

func Wrap(rpc rpcclient.Client) PaladinClient {
	return &paladinClient{
		Client:                 rpc,
		receiptPollingInterval: DefaultReceiptPollingInterval,
	}
}

func New() PaladinClient {
	return Wrap(&unconnectedRPC{})
}

func (c *paladinClient) HTTP(ctx context.Context, conf *pldconf.HTTPClientConfig) (PaladinClient, error) {
	rpc, err := rpcclient.NewHTTPClient(ctx, conf)
	if err != nil {
		return nil, err
	}
	c.Client = rpc
	return c, nil
}

func (c *paladinClient) WebSocket(ctx context.Context, conf *pldconf.WSClientConfig) (PaladinClient, error) {
	rpc, err := rpcclient.NewWSClient(ctx, conf)
	if err != nil {
		return nil, err
	}
	c.Client = rpc
	return c, nil
}

type unconnectedRPC struct{}

func (u *unconnectedRPC) CallRPC(ctx context.Context, result interface{}, method string, params ...interface{}) rpcclient.ErrorRPC {
	return rpcclient.WrapErrorRPC(rpcclient.RPCCodeInternalError, i18n.NewError(ctx, tkmsgs.MsgPaladinClientNoConnection))
}

func (c *paladinClient) ReceiptPollingInterval(t time.Duration) PaladinClient {
	c.receiptPollingInterval = t
	return c
}
