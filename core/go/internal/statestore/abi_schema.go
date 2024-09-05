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
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type abiSchema struct {
	*SchemaPersisted
	tc           abi.TypeComponent
	definition   *abi.Parameter
	primaryType  string
	typeSet      eip712.TypeSet
	abiLabelInfo []*schemaLabelInfo
}

func newABISchema(ctx context.Context, domainID string, def *abi.Parameter) (*abiSchema, error) {
	as := &abiSchema{
		SchemaPersisted: &SchemaPersisted{
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
		as.SchemaPersisted.Signature, err = as.FullSignature(ctx)
	}
	if err == nil {
		as.ID = tktypes.Bytes32Keccak([]byte(as.SchemaPersisted.Signature))
	}
	if err != nil {
		return nil, err
	}
	return as, nil
}

func newABISchemaFromDB(ctx context.Context, persisted *SchemaPersisted) (*abiSchema, error) {
	as := &abiSchema{
		SchemaPersisted: persisted,
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

func (as *abiSchema) IDString() string {
	return as.SchemaPersisted.ID.String()
}

func (as *abiSchema) Signature() string {
	return as.SchemaPersisted.Signature
}

func (as *abiSchema) Persisted() *SchemaPersisted {
	return as.SchemaPersisted
}

func (as *abiSchema) labelInfo() []*schemaLabelInfo {
	return as.abiLabelInfo
}

// Build the TypedDataV4 signature of the struct, from the ABI definition
// Note the "internalType" field of form "struct SomeTypeName" is required for
// nested tuple types in the ABI.
func (as *abiSchema) typedDataV4Setup(ctx context.Context, isNew bool) (err error) {
	if as.definition.Type != "tuple" || as.definition.InternalType == "" {
		return i18n.NewError(ctx, msgs.MsgStateABITypeMustBeTuple)
	}
	if as.tc, err = as.definition.TypeComponentTreeCtx(ctx); err != nil {
		return err
	}
	as.primaryType, as.typeSet, err = eip712.ABItoTypedDataV4(ctx, as.tc)
	if err == nil {
		err = as.labelSetup(ctx, isNew)
	}
	return err
}

func (as *abiSchema) labelSetup(ctx context.Context, isNew bool) error {
	labelIndex := 0
	uniqueMap := map[string]bool{}
	for i, tc := range as.tc.TupleChildren() {
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
			as.abiLabelInfo = append(as.abiLabelInfo, &schemaLabelInfo{
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

type parsedStateData struct {
	jsonTree    interface{}
	cv          *abi.ComponentValue
	labels      []*StateLabel
	int64Labels []*StateInt64Label
	labelValues filters.PassthroughValueSet
}

func (as *abiSchema) parseStateData(ctx context.Context, data tktypes.RawJSON) (*parsedStateData, error) {
	var psd parsedStateData
	err := json.Unmarshal([]byte(data), &psd.jsonTree)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateInvalidValue)
	}
	psd.cv, err = as.tc.ParseExternalCtx(ctx, psd.jsonTree)
	if err != nil {
		return nil, err
	}

	psd.labelValues = make(filters.PassthroughValueSet)
	for _, fieldName := range as.Labels {
		matched := false
		for _, f := range psd.cv.Children {
			if f.Component.KeyName() == fieldName {
				textLabel, int64Label, err := as.buildLabel(ctx, fieldName, f)
				if err != nil {
					return nil, err
				}
				if textLabel != nil {
					psd.labels = append(psd.labels, textLabel)
					psd.labelValues[textLabel.Label] = textLabel.Value
				} else {
					psd.int64Labels = append(psd.int64Labels, int64Label)
					psd.labelValues[int64Label.Label] = int64Label.Value
				}
				matched = true
				break
			}
		}
		if !matched {
			return nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldMissing, fieldName)
		}
	}
	return &psd, nil
}

// Take the state, parse the value into the type tree of this schema, and from that
// build the label values to store in the DB for comparison appropriate to the type.
func (as *abiSchema) ProcessState(ctx context.Context, data tktypes.RawJSON) (*StateWithLabels, error) {

	psd, err := as.parseStateData(ctx, data)
	if err != nil {
		return nil, err
	}

	// Now do a typed data v4 hash of the struct value itself
	hash, err := eip712.HashStruct(ctx, as.primaryType, psd.jsonTree, as.typeSet)

	// We need to re-serialize the data according to the ABI to:
	// - Ensure it's valid
	// - Remove anything that is not part of the schema
	// - Standardize formatting of all the data elements so domains do not need to worry
	var jsonData []byte
	if err == nil {
		jsonData, err = tktypes.StandardABISerializer().SerializeJSONCtx(ctx, psd.cv)
	}
	if err != nil {
		return nil, err
	}

	hashID := tktypes.NewBytes32FromSlice(hash)
	for i := range psd.labels {
		psd.labels[i].State = hashID
	}
	for i := range psd.int64Labels {
		psd.int64Labels[i].State = hashID
	}
	now := tktypes.TimestampNow()
	return &StateWithLabels{
		State: &State{
			ID:          hashID,
			CreatedAt:   now,
			DomainID:    as.DomainID,
			Schema:      as.ID,
			Data:        jsonData,
			Labels:      psd.labels,
			Int64Labels: psd.int64Labels,
		},
		LabelValues: addStateBaseLabels(psd.labelValues, hashID, now),
	}, nil
}

func (as *abiSchema) RecoverLabels(ctx context.Context, s *State) (*StateWithLabels, error) {
	psd, err := as.parseStateData(ctx, s.Data)
	if err != nil {
		return nil, err
	}
	return &StateWithLabels{
		State:       s,
		LabelValues: addStateBaseLabels(psd.labelValues, s.ID, s.CreatedAt),
	}, nil
}
