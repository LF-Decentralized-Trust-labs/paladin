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
	"sort"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/pldmsgs"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
)

type PaladinClient interface {
	// Direct RPC access
	rpcclient.Client

	// Config
	ReceiptPollingInterval(t time.Duration) PaladinClient
	HTTP(ctx context.Context, conf *pldconf.HTTPClientConfig) (PaladinClient, error)
	WebSocket(ctx context.Context, conf *pldconf.WSClientConfig) (PaladinWSClient, error)

	// High level transaction building and submission APIs
	TxBuilder(ctx context.Context) TxBuilder

	// Quick access to TxBuilder(ctx).ABI(a)
	ForABI(ctx context.Context, a abi.ABI) TxBuilder

	// Paladin transaction RPC interface
	PTX() PTX

	// Paladin Key Manager RPC interface
	KeyManager() KeyManager

	// Paladin Transport RPC interface
	Transport() Transport

	// Paladin Registry RPC interface
	Registry() Registry

	// Paladin state store RPC interface
	StateStore() StateStore

	// Paladin block index
	BlockIndex() BlockIndex

	// Paladin pgroup RPC interface
	PrivacyGroups() PrivacyGroups
}

type RPCModule interface {
	Group() string
	Methods() []string
	MethodInfo(method string) *RPCMethodInfo
}

type RPCMethodInfo struct {
	Inputs []string
	Output string
}

type RPCSubscriptionInfo struct {
	rpcclient.SubscriptionConfig
	FixedInputs []string
	Inputs      []string
}

type rpcModuleInfo struct {
	group         string
	methodInfo    map[string]RPCMethodInfo
	subscriptions []RPCSubscriptionInfo
}

func (fg *rpcModuleInfo) Group() string {
	return fg.group
}

func (fg *rpcModuleInfo) Methods() []string {
	methods := make([]string, 0, len(fg.methodInfo))
	for name := range fg.methodInfo {
		methods = append(methods, name)
	}
	sort.Strings(methods) // needs to be a consistent order
	return methods
}

func (fg *rpcModuleInfo) MethodInfo(method string) *RPCMethodInfo {
	info, found := fg.methodInfo[method]
	if !found {
		return nil
	}
	return &info
}

type PaladinWSClient interface {
	PaladinClient
	Close()
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

func WrapRestyClient(rc *resty.Client) PaladinClient {
	return Wrap(rpcclient.WrapRestyClient(rc))
}

func New() PaladinClient {
	return Wrap(&unconnectedRPC{})
}

func (c *paladinClient) WSClient(ctx context.Context) (rpcclient.WSClient, error) {
	wsc, ok := c.Client.(rpcclient.WSClient)
	if !ok {
		return nil, i18n.NewError(ctx, pldmsgs.MsgPaladinClientWebSocketRequired)
	}
	return wsc, nil
}

func (c *paladinClient) HTTP(ctx context.Context, conf *pldconf.HTTPClientConfig) (PaladinClient, error) {
	rpc, err := rpcclient.NewHTTPClient(ctx, conf)
	if err != nil {
		return nil, err
	}
	c.Client = rpc
	return c, nil
}

func (c *paladinClient) WebSocket(ctx context.Context, conf *pldconf.WSClientConfig) (PaladinWSClient, error) {
	rpc, err := rpcclient.NewWSClient(ctx, conf)
	if err == nil {
		err = rpc.Connect(ctx)
	}
	if err != nil {
		return nil, err
	}
	c.Client = rpc
	return &wsPaladinClient{paladinClient: c, wsRPC: rpc}, nil
}

type unconnectedRPC struct{}

func (u *unconnectedRPC) CallRPC(ctx context.Context, result interface{}, method string, params ...interface{}) rpcclient.ErrorRPC {
	return rpcclient.NewRPCError(ctx, rpcclient.RPCCodeInternalError, pldmsgs.MsgPaladinClientNoConnection)
}

func (c *paladinClient) ReceiptPollingInterval(t time.Duration) PaladinClient {
	c.receiptPollingInterval = t
	return c
}

type wsPaladinClient struct {
	*paladinClient
	wsRPC rpcclient.WSClient
}

func (wsc *wsPaladinClient) Close() {
	wsc.wsRPC.Close()
}
