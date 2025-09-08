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

package zetosignerapi

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
)

// StaticKeyEntryConfig is the configuration for a ZK prover
// based on SNARK, which typically takes a circuit and proving key
type SnarkProverConfig struct {
	signerapi.ConfigNoExt
	CircuitsDir         string `json:"circuitsDir"`         // directory for the circuits runtime (WASM currently supported)
	ProvingKeysDir      string `json:"provingKeysDir"`      // public parameters for the prover, specific to each circuit
	MaxProverPerCircuit *int   `json:"maxProverPerCircuit"` // maximum number of proving runtime per circuit, each prover owns a standalone WASM instance
}

type CircuitType string

const (
	Deposit        CircuitType = "deposit"
	Withdraw       CircuitType = "withdraw"
	Transfer       CircuitType = "transfer"
	TransferLocked CircuitType = "transferLocked"
)

type Circuit struct {
	Name           string      `yaml:"name" json:"name"`
	Type           CircuitType `yaml:"type" json:"type"`
	UsesNullifiers bool        `yaml:"usesNullifiers" json:"usesNullifiers"`
	UsesEncryption bool        `yaml:"usesEncryption" json:"usesEncryption"`
	UsesKyc        bool        `yaml:"usesKyc" json:"usesKyc"`
}

func (c *Circuit) ToProto() *proto.Circuit {
	return &proto.Circuit{
		Name:           c.Name,
		Type:           string(c.Type),
		UsesNullifiers: c.UsesNullifiers,
		UsesEncryption: c.UsesEncryption,
		UsesKyc:        c.UsesKyc,
	}
}

type Circuits map[string]*Circuit

func (cs Circuits) Init() {
	for circuitType, circuit := range cs {
		circuit.Type = CircuitType(circuitType)
	}
}

func NewCircuitFromProto(pb *proto.Circuit) *Circuit {
	return &Circuit{
		Name:           pb.Name,
		Type:           CircuitType(pb.Type),
		UsesNullifiers: pb.UsesNullifiers,
		UsesEncryption: pb.UsesEncryption,
		UsesKyc:        pb.UsesKyc,
	}
}

// Implements the extensible config interface of the signer
var _ signerapi.ExtensibleConfig = &SnarkProverConfig{}

func (c *SnarkProverConfig) KeyStoreConfig() *pldconf.KeyStoreConfig {
	return &c.KeyStore
}

func (c *SnarkProverConfig) KeyDerivationConfig() *pldconf.KeyDerivationConfig {
	return &c.KeyDerivation
}
