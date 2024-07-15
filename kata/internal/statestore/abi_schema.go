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

package statestore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/eip712"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

type ABISchema interface {
	Schema
	Definition() *abi.Parameter
}

type abiSchema struct {
	persisted  *SchemaEntity
	definition *abi.Parameter
}

func NewABISchema(ctx context.Context, def *abi.Parameter) (ABISchema, error) {
	as := &abiSchema{
		persisted:  &SchemaEntity{Type: SchemaTypeABI, Labels: []string{}},
		definition: def,
	}
	abiJSON, err := json.Marshal(def)
	if err == nil {
		as.persisted.Content = string(abiJSON)
		for _, p := range def.Components {
			if p.Indexed {
				as.persisted.Labels = append(as.persisted.Labels, p.Name)
			}
		}
		as.persisted.Signature, err = as.FullSignature(ctx)
	}
	if err == nil {
		as.persisted.Hash = *HashIDKeccak([]byte(as.persisted.Signature))
	}
	if err != nil {
		return nil, err
	}
	return as, nil
}

func newABISchemaFromDB(ctx context.Context, persisted *SchemaEntity) (ABISchema, error) {
	as := &abiSchema{
		persisted: persisted,
	}
	err := json.Unmarshal([]byte(persisted.Content), &as.definition)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateInvalidABIParam)
	}
	return as, nil
}

func (as *abiSchema) Type() SchemaType {
	return SchemaTypeABI
}

func (as *abiSchema) Persisted() *SchemaEntity {
	return as.persisted
}

func (as *abiSchema) Definition() *abi.Parameter {
	return as.definition
}

// Build the TypedDataV4 signature of the struct, from the ABI definition
// Note the "internalType" field of form "struct SomeTypeName" is required for
// nested tuple types in the ABI.
func (as *abiSchema) TypedDataV4Signature(ctx context.Context) (string, error) {
	tc, err := as.definition.TypeComponentTreeCtx(ctx)
	if err != nil {
		return "", err
	}
	primaryType, typeSet, err := eip712.ABItoTypedDataV4(ctx, tc)
	if err != nil {
		return "", err
	}
	return typeSet.Encode(primaryType), nil
}

func (as *abiSchema) FullSignature(ctx context.Context) (string, error) {
	typeSig, err := as.TypedDataV4Signature(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("type=%s,labels=[%s]", typeSig, strings.Join(as.persisted.Labels, ",")), nil
}
