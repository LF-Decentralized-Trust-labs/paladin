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

package main

import (
	"context"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type PrivateSmartContract struct {
	DeployTX      uuid.UUID        `json:"deployTransaction"   gorm:"column:deploy_tx"`
	DomainAddress types.EthAddress `json:"domainAddress"       gorm:"column:domain_address"`
	Address       types.EthAddress `json:"address"             gorm:"column:address"`
	ConfigBytes   types.HexBytes   `json:"configBytes"         gorm:"column:config_bytes"`
}

type domainContract struct {
	d    *domain
	info *PrivateSmartContract
}

func (d *domain) GetSmartContractByAddress(ctx context.Context, addr types.EthAddress) (components.DomainSmartContract, error) {
	dc, isCached := d.contractCache.Get(addr)
	if isCached {
		return dc, nil
	}

	var contracts []*PrivateSmartContract
	err := d.dm.persistence.DB().
		Table("private_smart_contracts").
		Where("domain_address = ?", d.factoryContractAddress).
		Where("address = ?", addr).
		WithContext(ctx).
		Limit(1).
		Find(&contracts).
		Error
	if err != nil {
		return nil, err
	}
	if len(contracts) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgDomainContractNotFoundByAddr, addr)
	}

	dc = &domainContract{
		d:    d,
		info: contracts[0],
	}
	d.contractCache.Set(addr, dc)
	return dc, nil
}

func (dc *domainContract) Domain() components.Domain {
	return dc.d
}
