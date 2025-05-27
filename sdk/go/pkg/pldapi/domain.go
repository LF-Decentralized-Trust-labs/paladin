// Copyright © 2025 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pldapi

import (
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
)

type Domain struct {
	Name            string               `docstruct:"Domain" json:"name"`
	RegistryAddress *pldtypes.EthAddress `docstruct:"Domain" json:"registryAddress"`
}

type DomainSmartContract struct {
	DomainName    string               `docstruct:"SmartContract" json:"domainName"`
	DomainAddress *pldtypes.EthAddress `docstruct:"SmartContract" json:"domainAddress"`
	Address       pldtypes.EthAddress  `docstruct:"SmartContract" json:"address"`
}
