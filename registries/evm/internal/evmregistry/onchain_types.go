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

package evmregistry

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type SolidityBuild struct {
	ABI      abi.ABI          `json:"abi"`
	Bytecode tktypes.HexBytes `json:"bytecode"`
}

type identityRegistryContractDefinition struct {
	abi                         abi.ABI
	identityRegisteredSignature tktypes.Bytes32
	propertySetSignature        tktypes.Bytes32
}

const identityRegisteredEventSolSig = "event IdentityRegistered(bytes32 parentIdentityHash, bytes32 identityHash, string name, address owner)"

type IdentityRegisteredEvent struct {
	ParentIdentityHash tktypes.Bytes32    `json:"parentIdentityHash"`
	IdentityHash       tktypes.Bytes32    `json:"identityHash"`
	Name               string             `json:"name"`
	Owner              tktypes.EthAddress `json:"owner"`
}

const propertySetEventSolSig = "event PropertySet(bytes32 identityHash, string name, string value)"

type PropertySetEvent struct {
	IdentityHash tktypes.Bytes32 `json:"identityHash"`
	Name         string          `json:"name"`
	Value        string          `json:"value"`
}

func mustLoadIdentityRegistryContractDetail(buildOutput []byte) *identityRegistryContractDefinition {
	var build SolidityBuild
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}

	// We require the event not to have changed in it's signature (or our type parsing will fail)

	identityRegisteredEvent := build.ABI.Events()["IdentityRegistered"]
	if identityRegisteredEvent.SolString() != identityRegisteredEventSolSig {
		panic(fmt.Sprintf("contract signature has changed: %s", identityRegisteredEvent.SolString()))
	}

	propertySetEvent := build.ABI.Events()["PropertySet"]
	if propertySetEvent.SolString() != propertySetEventSolSig {
		panic(fmt.Sprintf("contract signature has changed: %s", propertySetEvent.SolString()))
	}

	return &identityRegistryContractDefinition{
		abi:                         build.ABI,
		identityRegisteredSignature: tktypes.Bytes32(identityRegisteredEvent.SignatureHashBytes()),
		propertySetSignature:        tktypes.Bytes32(propertySetEvent.SignatureHashBytes()),
	}
}
