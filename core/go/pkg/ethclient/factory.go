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

package ethclient

import (
	"context"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
)

// Allows separate components to maintain separate connections/connection-pools to the
// blockchain, all using a common set of configuration pointing at the same blockchain.
type EthClientFactoryBase interface {
	Start() error   // connects the shared websocket and queries the chainID
	Stop()          // closes HTTP client and shared WS client
	ChainID() int64 // available after start
}

type EthClientFactory interface {
	EthClientFactoryBase
	HTTPClient() EthClient     // HTTP client
	SharedWS() EthClient       // WS client with a single long lived socket shared across multiple components
	NewWS() (EthClient, error) // created a dedicated socket - which the caller responsible for closing
}

type EthClientFactoryWithKeyManager interface {
	EthClientFactoryBase
	HTTPClient() EthClientWithKeyManager     // HTTP client
	SharedWS() EthClientWithKeyManager       // WS client with a single long lived socket shared across multiple components
	NewWS() (EthClientWithKeyManager, error) // created a dedicated socket - which the caller responsible for closing
}

type ethClientFactory struct {
	bgCtx context.Context

	conf   *pldconf.EthClientConfig
	keymgr KeyManager

	httpRPC    rpcclient.Client
	httpClient *ethClient

	sharedWSClient *ethClient

	wsConf *pldconf.WSClientConfig

	chainID int64
}

type ethClientFactoryKeyManagerWrapper struct {
	ecf *ethClientFactory
}

// During construction the shared WS connection is established, and the ChainID is queried
// using that connection.
//
// Callers can later
func NewEthClientFactory(bgCtx context.Context, conf *pldconf.EthClientConfig) (_ EthClientFactory, err error) {
	return newEthClientFactory(bgCtx, nil, conf)
}

func NewEthClientFactoryWithKeyManager(bgCtx context.Context, keymgr KeyManager, conf *pldconf.EthClientConfig) (_ EthClientFactoryWithKeyManager, err error) {
	ecf, err := newEthClientFactory(bgCtx, keymgr, conf)
	if err != nil {
		return nil, err
	}
	return &ethClientFactoryKeyManagerWrapper{ecf}, nil
}

func newEthClientFactory(bgCtx context.Context, keymgr KeyManager, conf *pldconf.EthClientConfig) (_ *ethClientFactory, err error) {
	ecf := &ethClientFactory{
		bgCtx:   bgCtx,
		conf:    conf,
		keymgr:  keymgr,
		chainID: -1,
	}
	// Parse the HTTP and build the HTTP client - we only have one of these across the factory
	// as within the HTTP client there are as many connections as required for parallelism
	if conf.HTTP.URL == "" {
		return nil, i18n.NewError(bgCtx, msgs.MsgEthClientHTTPURLMissing)
	}
	if ecf.httpRPC, err = rpcclient.NewHTTPClient(bgCtx, &conf.HTTP); err != nil {
		return nil, err
	}

	// Move onto WS, which can re-use the HTTP URL if required
	if conf.WS.URL == "" {
		noHTTPPrefix, trimmed := strings.CutPrefix(conf.HTTP.URL, "http")
		if trimmed {
			conf.WS.URL = "ws" + noHTTPPrefix
		}
	}
	err = rpcclient.ValidateWSClientConfig(bgCtx, &conf.WS)
	if err != nil {
		return nil, err
	}
	ecf.wsConf = &conf.WS

	return ecf, nil
}

func (ecf *ethClientFactory) Start() (err error) {
	// Connect and check the two connections are to the same network
	var sharedWSClient EthClient
	httpClient, err := WrapRPCClient(ecf.bgCtx, ecf.keymgr, ecf.httpRPC, ecf.conf)
	if err == nil {
		sharedWSClient, err = ecf.NewWS()
	}
	if err != nil {
		return err
	}
	ecf.httpClient = httpClient.(*ethClient)
	ecf.sharedWSClient = sharedWSClient.(*ethClient)
	httpChainID := ecf.httpClient.ChainID()
	wsChainID := ecf.sharedWSClient.ChainID()
	if wsChainID != httpChainID {
		return i18n.NewError(ecf.bgCtx, msgs.MsgEthClientChainIDMismatch, httpChainID, wsChainID)
	}
	ecf.chainID = httpChainID
	return err
}

func (ecf *ethClientFactory) NewWS() (ec EthClient, err error) {
	wsRPC := rpcclient.WrapWSConfig(ecf.wsConf)
	err = wsRPC.Connect(ecf.bgCtx)
	if err == nil {
		ec, err = WrapRPCClient(ecf.bgCtx, ecf.keymgr, wsRPC, ecf.conf)
	}
	return ec, err
}

func (ecf *ethClientFactory) HTTPClient() EthClient {
	return ecf.httpClient
}

func (ecf *ethClientFactory) SharedWS() EthClient {
	if ecf.sharedWSClient == nil {
		panic("call to SharedWS() before Start")
	}
	return ecf.sharedWSClient
}

func (ecf *ethClientFactory) Stop() {
	ecf.httpClient.Close()
	ecf.sharedWSClient.Close()
}

func (ecf *ethClientFactory) ChainID() int64 {
	return ecf.chainID
}

// Wrapper for key manager support in environments using this directly to access the blockchain rather than in Paladin

func (w *ethClientFactoryKeyManagerWrapper) Start() error {
	return w.ecf.Start()
}

func (w *ethClientFactoryKeyManagerWrapper) NewWS() (EthClientWithKeyManager, error) {
	ec, err := w.ecf.NewWS()
	if err != nil {
		return nil, err
	}
	return ec.(EthClientWithKeyManager), nil
}

func (w *ethClientFactoryKeyManagerWrapper) HTTPClient() EthClientWithKeyManager {
	return w.ecf.HTTPClient().(EthClientWithKeyManager)
}

func (w *ethClientFactoryKeyManagerWrapper) SharedWS() EthClientWithKeyManager {
	return w.ecf.SharedWS().(EthClientWithKeyManager)
}

func (w *ethClientFactoryKeyManagerWrapper) Stop() {
	w.ecf.Stop()
}

func (w *ethClientFactoryKeyManagerWrapper) ChainID() int64 {
	return w.ecf.ChainID()
}
