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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
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
	persisted   *SchemaEntity
	definition  *abi.Parameter
	primaryType string
	typeSet     eip712.TypeSet
}

func NewABISchema(ctx context.Context, domainID string, def *abi.Parameter) (ABISchema, error) {
	as := &abiSchema{
		persisted: &SchemaEntity{
			DomainID:      domainID,
			Type:          SchemaTypeABI,
			TextLabels:    []string{},
			IntegerLabels: []string{},
		},
		definition: def,
	}
	abiJSON, err := json.Marshal(def)
	if err == nil {
		as.persisted.Content = string(abiJSON)
		for i, p := range def.Components {
			if p.Indexed {
				if len(p.Name) == 0 {
					return nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldNotNamed, i)
				}
				tc, err := p.TypeComponentTreeCtx(ctx)
				if err != nil {
					return nil, err
				}
				et := tc.ElementaryType()
				if et == nil {
					return nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldNotElementary, p.Name, tc.String())
				}
				baseType := et.BaseType()
				switch baseType {
				case abi.BaseTypeInt, abi.BaseTypeUInt:
					if baseType == abi.BaseTypeInt && tc.ElementaryM() > 64 ||
						baseType == abi.BaseTypeUInt && tc.ElementaryM() >= 64 {
						// To big to fit into a signed 8 byte int64 value - must fall back to text index
						as.persisted.TextLabels = append(as.persisted.TextLabels, p.Name)
					} else {
						as.persisted.IntegerLabels = append(as.persisted.IntegerLabels, p.Name)
					}
				case abi.BaseTypeAddress, abi.BaseTypeBytes, abi.BaseTypeFunction, abi.BaseTypeString:
					as.persisted.TextLabels = append(as.persisted.TextLabels, p.Name)
				default:
					return nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnsupportedType, p.Name, tc.String())
				}
			}
		}
		err = as.typedDataV4Setup(ctx)
	}
	if err == nil {
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
	err = as.typedDataV4Setup(ctx)
	if err != nil {
		return nil, err
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
func (as *abiSchema) typedDataV4Setup(ctx context.Context) error {
	tc, err := as.definition.TypeComponentTreeCtx(ctx)
	if err != nil {
		return err
	}
	as.primaryType, as.typeSet, err = eip712.ABItoTypedDataV4(ctx, tc)
	return err
}

func (as *abiSchema) FullSignature(ctx context.Context) (string, error) {
	typeSig := as.typeSet.Encode(as.primaryType)
	return fmt.Sprintf("type=%s,tLabels=[%s],iLabels=[%s]", typeSig,
		strings.Join(as.persisted.TextLabels, ","), strings.Join(as.persisted.IntegerLabels, ",")), nil
}

// Take the state, parse the value into the type tree of this schema, and from that
// build the label values to store in the DB for comparison appropriate to the type.
func (as *abiSchema) ProcessState(ctx context.Context, s *State) error {

	tc, err := as.definition.TypeComponentTreeCtx(ctx)
	if err != nil {
		return err
	}
	var jsonTree interface{}
	err = json.Unmarshal([]byte(s.Data), &jsonTree)
	if err != nil {
		return err
	}
	cv, err := tc.ParseExternalCtx(ctx, jsonTree)
	if err != nil {
		return err
	}

	textLabels := make([]StateTextLabel, 0, len(as.persisted.TextLabels))
	for _, fieldName := range as.persisted.TextLabels {
		matched := false
		for _, f := range cv.Children {
			if f.Component.KeyName() == fieldName {
				et := f.Component.ElementaryType()
				if et == nil {
					return i18n.NewError(ctx, msgs.MsgStateLabelFieldNotElementary, fieldName, f.Component.String())
				}
				switch vt := f.Value.(type) {
				case *big.Int:
					textLabels = append(textLabels, StateTextLabel{
						Label: fieldName,
						Value: "0x" + vt.Text(16), // we store a hex encoded string - not sortable if a text field is used, but no max value
					})
				case []byte:
					textLabels = append(textLabels, StateTextLabel{
						Label: fieldName,
						Value: "0x" + hex.EncodeToString(vt),
					})
				case string:
					textLabels = append(textLabels, StateTextLabel{
						Label: fieldName,
						Value: vt,
					})
				default:
					return i18n.NewError(ctx, msgs.MsgStateLabelFieldUnsupportedType, fieldName, et.String())
				}
				matched = true
				break
			}
		}
		if !matched {
			return i18n.NewError(ctx, msgs.MsgStateLabelFieldMissing, fieldName)
		}
	}

	integerLabels := make([]StateIntegerLabel, 0, len(as.persisted.IntegerLabels))
	for _, fieldName := range as.persisted.IntegerLabels {
		matched := false
		for _, f := range cv.Children {
			if f.Component.KeyName() == fieldName {
				et := f.Component.ElementaryType()
				if et == nil {
					return i18n.NewError(ctx, msgs.MsgStateLabelFieldNotElementary, fieldName, f.Component.String())
				}
				switch vt := f.Value.(type) {
				case *big.Int:
					integerLabels = append(integerLabels, StateIntegerLabel{
						Label: fieldName,
						Value: vt.Int64(),
					})
				default:
					return i18n.NewError(ctx, msgs.MsgStateLabelFieldNotElementary, fieldName, f.Component.String())
				}
				matched = true
				break
			}
		}
		if !matched {
			return i18n.NewError(ctx, msgs.MsgStateLabelFieldMissing, fieldName)
		}
	}

	// Now do a typed data v4 hash of the struct value itself
	hash, err := eip712.HashStruct(ctx, as.primaryType, jsonTree, as.typeSet)
	if err != nil {
		return err
	}

	s.Hash = *NewHashIDSlice32(hash)
	for i := range textLabels {
		textLabels[i].State = s.Hash
	}
	s.TextLabels = textLabels
	for i := range integerLabels {
		integerLabels[i].State = s.Hash
	}
	s.IntegerLabels = integerLabels

	return nil
}
