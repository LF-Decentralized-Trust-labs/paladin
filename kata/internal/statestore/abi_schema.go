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
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type abiSchema struct {
	*SchemaEntity
	definition  *abi.Parameter
	primaryType string
	typeSet     eip712.TypeSet
	labelInfo   []*schemaLabelInfo
}

func newABISchema(ctx context.Context, domainID string, def *abi.Parameter) (*abiSchema, error) {
	as := &abiSchema{
		SchemaEntity: &SchemaEntity{
			DomainID: domainID,
			Type:     SchemaTypeABI,
			Labels:   []string{},
		},
		definition: def,
	}
	abiJSON, err := json.Marshal(def)
	if err == nil {
		as.Definition = abiJSON
		err = as.typedDataV4Setup(ctx, true)
	}
	if err == nil {
		as.Signature, err = as.FullSignature(ctx)
	}
	if err == nil {
		as.Hash = *HashIDKeccak([]byte(as.Signature))
	}
	if err != nil {
		return nil, err
	}
	return as, nil
}

func newABISchemaFromDB(ctx context.Context, persisted *SchemaEntity) (*abiSchema, error) {
	as := &abiSchema{
		SchemaEntity: persisted,
	}
	err := json.Unmarshal(persisted.Definition, &as.definition)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateInvalidSchema)
	}
	err = as.typedDataV4Setup(ctx, false)
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (as *abiSchema) Type() SchemaType {
	return SchemaTypeABI
}

func (as *abiSchema) Persisted() *SchemaEntity {
	return as.SchemaEntity
}

func (as *abiSchema) LabelInfo() []*schemaLabelInfo {
	return as.labelInfo
}

// Build the TypedDataV4 signature of the struct, from the ABI definition
// Note the "internalType" field of form "struct SomeTypeName" is required for
// nested tuple types in the ABI.
func (as *abiSchema) typedDataV4Setup(ctx context.Context, isNew bool) error {
	if as.definition.Type != "tuple" || as.definition.InternalType == "" {
		return i18n.NewError(ctx, msgs.MsgStateABITypeMustBeTuple)
	}
	tc, err := as.definition.TypeComponentTreeCtx(ctx)
	if err != nil {
		return err
	}
	as.primaryType, as.typeSet, err = eip712.ABItoTypedDataV4(ctx, tc)
	if err == nil {
		err = as.labelSetup(ctx, tc, isNew)
	}
	return err
}

func (as *abiSchema) labelSetup(ctx context.Context, rootTC abi.TypeComponent, isNew bool) error {
	labelIndex := 0
	uniqueMap := map[string]bool{}
	for i, tc := range rootTC.TupleChildren() {
		p := tc.Parameter()
		if p.Indexed {
			if len(p.Name) == 0 {
				return i18n.NewError(ctx, msgs.MsgStateLabelFieldNotNamed, i)
			}
			if _, exists := uniqueMap[p.Name]; exists {
				return i18n.NewError(ctx, msgs.MsgStateLabelFieldNotUnique, i, p.Name)
			}
			uniqueMap[p.Name] = true
			labelType, labelResolver, err := as.getLabelResolver(ctx, labelIndex, p.Name, tc)
			if err != nil {
				return err
			}
			as.labelInfo = append(as.labelInfo, &schemaLabelInfo{
				label:         p.Name,
				virtualColumn: fmt.Sprintf("l%d", labelIndex),
				labelType:     labelType,
				resolver:      labelResolver,
			})
			if isNew {
				as.Labels = append(as.Labels, p.Name)
			}
			labelIndex++
		}
	}
	return nil
}

func (as *abiSchema) FullSignature(ctx context.Context) (string, error) {
	typeSig := as.typeSet.Encode(as.primaryType)
	return fmt.Sprintf("type=%s,labels=[%s]", typeSig, strings.Join(as.Labels, ",")), nil
}

func (as *abiSchema) getLabelType(ctx context.Context, fieldName string, tc abi.TypeComponent) (labelType, error) {
	if tc.ComponentType() != abi.ElementaryComponent {
		return -1, i18n.NewError(ctx, msgs.MsgStateLabelFieldNotElementary, fieldName, tc.String())
	}
	et := tc.ElementaryType()
	baseType := et.BaseType()
	switch baseType {
	case abi.BaseTypeInt:
		if tc.ElementaryM() <= 64 {
			// Up to signed int64 fits into an integer field in all DBs we currently support
			return labelTypeInt64, nil
		}
		// Otherwise we fall back to encoding as a fixed-width hex string - with a leading sign character
		return labelTypeInt256, nil
	case abi.BaseTypeUInt, abi.BaseTypeAddress /* address is uint160 really */ :
		if tc.ElementaryM() < 64 {
			// Up to signed uint63 fits into an integer field in all DBs we currently support (uint64 does not fit)
			return labelTypeInt64, nil
		}
		// Otherwise we fall back to encoding as a fixed-width hex string
		return labelTypeUint256, nil
	case abi.BaseTypeBytes, abi.BaseTypeFunction:
		return labelTypeBytes, nil
	case abi.BaseTypeString:
		return labelTypeString, nil
	case abi.BaseTypeBool:
		return labelTypeBool, nil
	default:
		return -1, i18n.NewError(ctx, msgs.MsgStateInvalidSchemaType, tc.String())
	}
}

func (as *abiSchema) buildLabel(ctx context.Context, fieldName string, f *abi.ComponentValue) (*StateLabel, *StateInt64Label, error) {
	labelType, err := as.getLabelType(ctx, fieldName, f.Component)
	if err != nil {
		return nil, nil, err
	}
	return as.mapValueToLabel(ctx, fieldName, labelType, f)
}

func (as *abiSchema) mapValueToLabel(ctx context.Context, fieldName string, labelType labelType, f *abi.ComponentValue) (*StateLabel, *StateInt64Label, error) {
	switch labelType {
	case labelTypeInt64:
		bigIntVal, ok := f.Value.(*big.Int)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, new(big.Int))
		}
		return nil, &StateInt64Label{Label: fieldName, Value: bigIntVal.Int64()}, nil
	case labelTypeInt256:
		bigIntVal, ok := f.Value.(*big.Int)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, new(big.Int))
		}
		// Otherwise we fall back to encoding as a fixed-width hex string - with a leading sign character
		filterString := filters.Int256ToFilterString(ctx, bigIntVal)
		return &StateLabel{Label: fieldName, Value: filterString}, nil, nil
	case labelTypeUint256:
		bigIntVal, ok := f.Value.(*big.Int)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, new(big.Int))
		}
		bigIntVal = bigIntVal.Abs(bigIntVal)
		filterString := filters.Uint256ToFilterString(ctx, bigIntVal)
		return &StateLabel{Label: fieldName, Value: filterString}, nil, nil
	case labelTypeBytes:
		byteValue, ok := f.Value.([]byte)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, []byte{})
		}
		// We do NOT use an 0x prefix on bytes types
		return &StateLabel{Label: fieldName, Value: hex.EncodeToString(byteValue)}, nil, nil
	case labelTypeString:
		strValue, ok := f.Value.(string)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, "")
		}
		return &StateLabel{Label: fieldName, Value: strValue}, nil, nil
	case labelTypeBool:
		bValue, ok := f.Value.(*big.Int)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, new(big.Int))
		}
		return nil, &StateInt64Label{Label: fieldName, Value: bValue.Int64()}, nil
	default:
		// Should not get here - if covered all the types above
		return nil, nil, i18n.NewError(ctx, msgs.MsgStateInvalidSchemaType, f.Component.String())
	}
}

func (as *abiSchema) getLabelResolver(ctx context.Context, labelIndex int, fieldName string, tc abi.TypeComponent) (labelType, filters.FieldResolver, error) {
	labelType, err := as.getLabelType(ctx, fieldName, tc)
	if err != nil {
		return -1, nil, err
	}
	sqlColumn := fmt.Sprintf("l%d.value", labelIndex)
	return as.mapLabelResolver(ctx, sqlColumn, labelType)
}

func (as *abiSchema) mapLabelResolver(ctx context.Context, sqlColumn string, labelType labelType) (labelType, filters.FieldResolver, error) {
	switch labelType {
	case labelTypeInt64:
		return labelType, filters.Int64Field(sqlColumn), nil
	case labelTypeInt256:
		return labelType, filters.Int256Field(sqlColumn), nil
	case labelTypeUint256:
		return labelType, filters.Uint256Field(sqlColumn), nil
	case labelTypeBytes:
		return labelType, filters.HexBytesField(sqlColumn), nil
	case labelTypeString:
		return labelType, filters.StringField(sqlColumn), nil
	case labelTypeBool:
		return labelType, filters.Int64Field(sqlColumn), nil // stored in the int64 index
	default:
		// Should not get here - if covered all the types above
		return -1, nil, i18n.NewError(ctx, msgs.MsgStateInvalidSchemaType, sqlColumn)
	}
}

// Take the state, parse the value into the type tree of this schema, and from that
// build the label values to store in the DB for comparison appropriate to the type.
func (as *abiSchema) ProcessState(ctx context.Context, data types.RawJSON) (*State, error) {

	tc, err := as.definition.TypeComponentTreeCtx(ctx)
	if err != nil {
		return nil, err
	}
	var jsonTree interface{}
	err = json.Unmarshal([]byte(data), &jsonTree)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateInvalidValue)
	}
	cv, err := tc.ParseExternalCtx(ctx, jsonTree)
	if err != nil {
		return nil, err
	}

	var labels []*StateLabel
	var int64Labels []*StateInt64Label
	for _, fieldName := range as.Labels {
		matched := false
		for _, f := range cv.Children {
			if f.Component.KeyName() == fieldName {
				textLabel, int64Label, err := as.buildLabel(ctx, fieldName, f)
				if err != nil {
					return nil, err
				}
				if textLabel != nil {
					labels = append(labels, textLabel)
				} else {
					int64Labels = append(int64Labels, int64Label)
				}
				matched = true
				break
			}
		}
		if !matched {
			return nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldMissing, fieldName)
		}
	}

	// Now do a typed data v4 hash of the struct value itself
	hash, err := eip712.HashStruct(ctx, as.primaryType, jsonTree, as.typeSet)

	// We need to re-serialize the data according to the ABI to:
	// - Ensure it's valid
	// - Remove anything that is not part of the schema
	// - Standardize formatting of all the data elements so domains do not need to worry
	var jsonData []byte
	if err == nil {
		jsonData, err = abi.NewSerializer().
			SetFormattingMode(abi.FormatAsObjects).
			SetIntSerializer(abi.Base10StringIntSerializer).
			SetFloatSerializer(abi.Base10StringFloatSerializer).
			SetByteSerializer(abi.HexByteSerializer0xPrefix).
			SerializeJSONCtx(ctx, cv)
	}
	if err != nil {
		return nil, err
	}

	hashID := *NewHashIDSlice32(hash)
	for i := range labels {
		labels[i].State = hashID
	}
	for i := range int64Labels {
		int64Labels[i].State = hashID
	}
	return &State{
		Hash:        hashID,
		DomainID:    as.DomainID,
		Schema:      as.Hash,
		Data:        jsonData,
		Labels:      labels,
		Int64Labels: int64Labels,
	}, nil
}
