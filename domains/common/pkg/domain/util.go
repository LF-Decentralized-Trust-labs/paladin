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

package domain

import (
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type SolidityBuild struct {
	ABI      abi.ABI                   `json:"abi"`
	Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
}

type SolidityBuildWithLinks struct {
	ABI            abi.ABI                                       `json:"abi"`
	Bytecode       string                                        `json:"bytecode"`
	LinkReferences map[string]map[string][]SolidityLinkReference `json:"linkReferences"`
}

type SolidityLinkReference struct {
	Start  int `json:"start"`
	Length int `json:"length"`
}

func LoadBuild(buildOutput []byte) *SolidityBuild {
	var build SolidityBuild
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}
	return &build
}

func LoadBuildLinked(buildOutput []byte, libraries map[string]string) *SolidityBuild {
	var build SolidityBuildWithLinks
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}
	bytecode, err := linkBytecode(build, libraries)
	if err != nil {
		panic(err)
	}
	return &SolidityBuild{
		ABI:      build.ABI,
		Bytecode: bytecode,
	}
}

// linkBytecode: performs linking by replacing placeholders with deployed addresses
// Based on a workaround from Hardhat team here:
// https://github.com/nomiclabs/hardhat/issues/611#issuecomment-638891597
func linkBytecode(artifact SolidityBuildWithLinks, libraries map[string]string) (ethtypes.HexBytes0xPrefix, error) {
	bytecode := artifact.Bytecode
	for _, fileReferences := range artifact.LinkReferences {
		for libName, fixups := range fileReferences {
			addr, found := libraries[libName]
			if !found {
				continue
			}
			for _, fixup := range fixups {
				start := 2 + fixup.Start*2
				end := start + fixup.Length*2
				bytecode = bytecode[0:start] + addr[2:] + bytecode[end:]
			}
		}
	}
	return hex.DecodeString(strings.TrimPrefix(bytecode, "0x"))
}

func FindVerifier(lookup string, verifiers []*pb.ResolvedVerifier) *pb.ResolvedVerifier {
	for _, verifier := range verifiers {
		if verifier.Lookup == lookup {
			return verifier
		}
	}
	return nil
}

func FindAttestation(name string, attestations []*pb.AttestationResult) *pb.AttestationResult {
	for _, attestation := range attestations {
		if attestation.Name == name {
			return attestation
		}
	}
	return nil
}
