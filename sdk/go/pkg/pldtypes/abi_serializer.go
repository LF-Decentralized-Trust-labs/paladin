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

package pldtypes

import (
	"context"
	"crypto/sha256"
	"net/url"
	"sort"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type JSONFormatOptions string

const DefaultJSONFormatOptions JSONFormatOptions = ""

func (jfo JSONFormatOptions) GetABISerializer(ctx context.Context) (serializer *abi.Serializer, err error) {
	return jfo.getABISerializer(ctx, false)
}

func (jfo JSONFormatOptions) GetABISerializerIgnoreErrors(ctx context.Context) *abi.Serializer {
	serializer, _ := jfo.getABISerializer(ctx, true)
	return serializer
}

func (jfo JSONFormatOptions) getABISerializer(ctx context.Context, skipErrors bool) (serializer *abi.Serializer, err error) {
	serializer = StandardABISerializer()
	if len(jfo) == 0 {
		return
	}
	options, err := url.ParseQuery(string(jfo))
	if err != nil {
		if !skipErrors {
			return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTypesInvalidJSONFormatOptions, jfo)
		}
	}
	for option, values := range options {
		for _, v := range values {
			switch strings.ToLower(option) {
			case "mode":
				switch strings.ToLower(v) {
				case "object":
					serializer = serializer.SetFormattingMode(abi.FormatAsObjects)
				case "array":
					serializer = serializer.SetFormattingMode(abi.FormatAsFlatArrays)
				case "self-describing":
					serializer = serializer.SetFormattingMode(abi.FormatAsSelfDescribingArrays)
				default:
					if !skipErrors {
						return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTypesUnknownJSONFormatOptions, option, v)
					}
				}
			case "number":
				switch strings.ToLower(v) {
				case "string": // default
					serializer = serializer.SetIntSerializer(abi.Base10StringIntSerializer)
				case "hex-0x", "hex":
					serializer = serializer.SetIntSerializer(abi.HexIntSerializer0xPrefix)
				case "json-number": // note consumer must be very careful to use a JSON parser that support large numbers
					serializer = serializer.SetIntSerializer(abi.JSONNumberIntSerializer)
				default:
					if !skipErrors {
						return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTypesUnknownJSONFormatOptions, option, v)
					}
				}
			case "bytes":
				switch strings.ToLower(v) {
				case "hex-0x", "hex":
					serializer = serializer.SetByteSerializer(abi.HexByteSerializer0xPrefix)
				case "hex-plain":
					serializer = serializer.SetByteSerializer(abi.HexByteSerializer)
				case "base64":
					serializer = serializer.SetByteSerializer(abi.Base64ByteSerializer)
				default:
					return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTypesUnknownJSONFormatOptions, option, v)
				}
			case "address":
				switch strings.ToLower(v) {
				case "hex-0x", "hex":
					serializer = serializer.SetAddressSerializer(abi.HexAddrSerializer0xPrefix)
				case "hex-plain":
					serializer = serializer.SetAddressSerializer(abi.HexAddrSerializerPlain)
				case "checksum":
					serializer = serializer.SetAddressSerializer(abi.ChecksumAddrSerializer)
				default:
					return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTypesUnknownJSONFormatOptions, option, v)
				}
			case "pretty":
				serializer = serializer.SetPretty(v != "false")
			default:
				if !skipErrors {
					return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTypesUnknownJSONFormatOptions, option, v)
				}
			}
		}
	}
	return serializer, nil
}

// The serializer we should use in all places that go from ABI validated data,
// back down to JSON that might be:
// 1) Passed to end-users over a JSON/RPC API
// 2) Passed to domain plugins over a gRPC API
func StandardABISerializer() *abi.Serializer {
	return abi.NewSerializer().
		SetFormattingMode(abi.FormatAsObjects).
		SetIntSerializer(abi.Base10StringIntSerializer).
		SetFloatSerializer(abi.Base10StringFloatSerializer).
		SetByteSerializer(abi.HexByteSerializer0xPrefix).
		SetAddressSerializer(abi.HexAddrSerializer0xPrefix)
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
				return i18n.NewError(ctx, pldmsgs.MsgTypesABIDefNotInBothStructs, sig)
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
				return i18n.NewError(ctx, pldmsgs.MsgTypesABIDefNotInBothStructs, sig)
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

func ABISolDefinitionHash(ctx context.Context, a abi.ABI, subMatch ...abi.EntryType) (*Bytes32, error) {
	hash := sha256.New()
	if len(subMatch) > 0 {
		subSelected := make(abi.ABI, 0, len(a))
		for _, e := range a {
			for _, t := range subMatch {
				if e.Type == t {
					subSelected = append(subSelected, e)
					break
				}
			}
		}
		a = subSelected
	}
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
