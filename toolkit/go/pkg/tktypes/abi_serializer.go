// Copyright © 2024 Kaleido, Inc.
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

package tktypes

import (
	"context"
	"crypto/sha256"
	"sort"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tkmsgs"
)

// The serializer we should use in all places that go from ABI validated data,
// back down to JSON that might be:
// 1) Stored in the state store
// 2) Passed to end-users over a JSON/RPC API
// 3) Passed to domain plugins over a gRPC API
func StandardABISerializer() *abi.Serializer {
	return abi.NewSerializer().
		SetFormattingMode(abi.FormatAsObjects).
		SetIntSerializer(abi.Base10StringIntSerializer).
		SetFloatSerializer(abi.Base10StringFloatSerializer).
		SetByteSerializer(abi.HexByteSerializer0xPrefix)
}

// Validates that two ABIs contains exactly the same entires
// - Includes names of types
// - Includes indexing and other modifiers
// - Allows any order of entries
func ABIsMustMatch(ctx context.Context, a, b abi.ABI, subMatch ...abi.EntryType) error {
	byDefsA, err := ABIBySolDefinition(ctx, a)
	if err != nil {
		return err
	}
	byDefsB, err := ABIBySolDefinition(ctx, b)
	if err != nil {
		return err
	}
	for sig, a := range byDefsA {
		mustMatch := len(subMatch) == 0
		for _, t := range subMatch {
			if a.Type == t {
				mustMatch = true
				break
			}
		}
		if mustMatch {
			if _, inB := byDefsB[sig]; !inB {
				return i18n.NewError(ctx, tkmsgs.MsgTypesABIDefNotInBothStructs, sig)
			}
			delete(byDefsB, sig)
		}
	}
	for sig, b := range byDefsB {
		mustMatch := len(subMatch) == 0
		for _, t := range subMatch {
			if b.Type == t {
				mustMatch = true
				break
			}
		}
		if mustMatch {
			if _, inA := byDefsA[sig]; !inA {
				return i18n.NewError(ctx, tkmsgs.MsgTypesABIDefNotInBothStructs, sig)
			}
		}
	}
	return nil
}

func ABIBySolDefinition(ctx context.Context, a abi.ABI) (map[string]*abi.Entry, error) {
	byDefs := make(map[string]*abi.Entry)
	for _, e := range a {
		solString, err := e.SolidityStringCtx(ctx)
		if err != nil {
			return nil, err
		}
		byDefs[solString] = e
	}
	return byDefs, nil
}

func ABISolDefinitionHash(ctx context.Context, a abi.ABI) (*Bytes32, error) {
	hash := sha256.New()
	bySolDef, err := ABIBySolDefinition(ctx, a)
	if err != nil {
		return nil, err
	}
	// Sort the strings to avoid ordering mattering in the hash
	solDefs := make([]string, 0, len(bySolDef))
	for solDef := range bySolDef {
		solDefs = append(solDefs, solDef)
	}
	sort.Strings(solDefs)
	// Hash the sorted strings
	for _, solDef := range solDefs {
		hash.Write([]byte(solDef))
	}
	h := Bytes32(hash.Sum(nil))
	return &h, nil
}
