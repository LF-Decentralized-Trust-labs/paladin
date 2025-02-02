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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tkmsgs"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Takes arbitrary JSON data and attempts to infer an ABI from it.
// Is not a complete system for all JSON.
// Current capability set:
// - Requires top-level JSON to be an object
// - Does NOT preserve order of properties in a JSON object
//   - This would require much more complex json.Token based code, or a swap-in library like
//
// - Supports simple types, objects, and arrays of same-typed values
// - Maps:
//   - strings -> string
//   - number  -> int256 (as long as input number is whole)
//   - boolean -> boolean
func ABIInferenceFromJSON(ctx context.Context, inputData RawJSON) (abi.ParameterArray, error) {

	var rootMap map[string]any
	dec := json.NewDecoder(bytes.NewReader(inputData))
	dec.UseNumber()
	err := dec.Decode(&rootMap)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, tkmsgs.MsgTypesInvalidJSONObjectForABIInference)
	}
	if len(rootMap) == 0 {
		return abi.ParameterArray{}, nil // Nil or empty input object case
	}
	return recurseInferParams(ctx, rootMap, true /* top level simple types marked indexed */)
}

func sortedMapKeys(m map[string]any) []string {
	sortedKeys := make([]string, 0, len(m))
	for k := range m {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	return sortedKeys
}

func recurseInferParams(ctx context.Context, in map[string]any, indexed bool) (abi.ParameterArray, error) {

	params := make(abi.ParameterArray, 0, len(in))
	for _, k := range sortedMapKeys(in) {
		switch v := in[k].(type) {
		case string:
			params = append(params, &abi.Parameter{
				Name:    k,
				Type:    "string",
				Indexed: indexed,
			})
		case json.Number:
			bf, ok := new(big.Float).SetString(v.String())
			if !ok || !bf.IsInt() {
				return nil, i18n.NewError(ctx, tkmsgs.MsgTypesNumberTypeInferenceRequiresInt, k, v)
			}
			params = append(params, &abi.Parameter{
				Name:    k,
				Type:    "int256",
				Indexed: indexed,
			})
		case bool:
			params = append(params, &abi.Parameter{
				Name:    k,
				Type:    "boolean",
				Indexed: indexed,
			})
		case []any:
			if len(v) == 0 {
				return nil, i18n.NewError(ctx, tkmsgs.MsgTypesCannotInferTypeOfEmptyArray, k)
			}
			// Infer recursively from the first parameter, giving it a pseudo name for error reporting
			pk := fmt.Sprintf("%s[]", k)
			nestedParams, err := recurseInferParams(ctx, map[string]any{pk: v[0]}, false)
			if err != nil {
				return nil, err
			}
			nestedParams[0].Name = k
			nestedParams[0].Type = fmt.Sprintf("%s[]", nestedParams[0].Type)
			params = append(params, nestedParams[0])
		case map[string]any:
			nestedParams, err := recurseInferParams(ctx, v, false)
			if err != nil {
				return nil, err
			}
			params = append(params, &abi.Parameter{
				Name:         k,
				Type:         "tuple",
				InternalType: fmt.Sprintf("struct %s", cases.Title(language.AmericanEnglish).String(k)),
				Components:   nestedParams,
			})
		default:
			return nil, i18n.NewError(ctx, tkmsgs.MsgTypesTypeInferenceNotSupportedForX, k, v)
		}
	}

	return params, nil

}
