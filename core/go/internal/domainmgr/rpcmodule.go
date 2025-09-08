/*
 * Copyright © 2025 Kaleido, Inc.
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

package domainmgr

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

func (dm *domainManager) buildRPCModule() {
	dm.rpcModule = rpcserver.NewRPCModule("domain").
		Add("domain_listDomains", dm.rpcQueryTransactions()).
		Add("domain_getDomain", dm.rpcGetDomain()).
		Add("domain_getDomainByAddress", dm.rpcGetDomainByAddress()).
		Add("domain_querySmartContracts", dm.rpcQuerySmartContracts()).
		Add("domain_getSmartContractByAddress", dm.rpcGetSmartContractByAddress())
}

func (dm *domainManager) rpcQueryTransactions() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context) ([]string, error) {
		res := []string{}
		for name := range dm.ConfiguredDomains() {
			res = append(res, name)
		}
		return res, nil
	})
}

func (dm *domainManager) rpcGetDomain() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, name string) (*pldapi.Domain, error) {
		domain, err := dm.getDomainByName(ctx, name)
		if err != nil {
			return nil, err
		}
		return &pldapi.Domain{
			Name:            domain.name,
			RegistryAddress: domain.registryAddress,
		}, nil
	})
}

func (dm *domainManager) rpcGetDomainByAddress() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, address pldtypes.EthAddress) (*pldapi.Domain, error) {
		domain, err := dm.getDomainByAddress(ctx, &address)
		if err != nil {
			return nil, err
		}
		return &pldapi.Domain{
			Name:            domain.name,
			RegistryAddress: domain.registryAddress,
		}, nil
	})
}

func (dm *domainManager) rpcQuerySmartContracts() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		query query.QueryJSON,
	) ([]*pldapi.DomainSmartContract, error) {
		return dm.querySmartContracts(ctx, &query)
	})
}

func (dm *domainManager) rpcGetSmartContractByAddress() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, address pldtypes.EthAddress) (*pldapi.DomainSmartContract, error) {
		var sc components.DomainSmartContract
		var err error
		err = dm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			sc, err = dm.GetSmartContractByAddress(ctx, dbTX, address)
			return err
		})
		return &pldapi.DomainSmartContract{
			DomainName:    sc.Domain().Name(),
			DomainAddress: sc.Domain().RegistryAddress(),
			Address:       sc.Address(),
		}, nil
	})
}
