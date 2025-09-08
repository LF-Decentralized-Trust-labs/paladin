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

package statemgr

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/eip712"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type abiSchema struct {
	*pldapi.Schema
	tc           abi.TypeComponent
	definition   *abi.Parameter
	primaryType  string
	typeSet      eip712.TypeSet
	abiLabelInfo []*schemaLabelInfo
}

func newABISchema(ctx context.Context, domainName string, def *abi.Parameter) (*abiSchema, error) {
	as := &abiSchema{
		Schema: &pldapi.Schema{
			DomainName: domainName,
			Type:       pldapi.SchemaTypeABI.Enum(),
			Labels:     []string{},
		},
		definition: def,
	}
	abiJSON, err := json.Marshal(def)
	if err == nil {
		as.Definition = abiJSON
		err = as.typedDataV4Setup(ctx, true)
	}
	if err == nil {
		as.Schema.Signature, err = as.FullSignature(ctx)
	}
	if err == nil {
		as.Schema.ID = pldtypes.Bytes32Keccak([]byte(as.Schema.Signature))
	}
	if err != nil {
		return nil, err
	}
	return as, nil
}

func newABISchemaFromDB(ctx context.Context, persisted *pldapi.Schema) (*abiSchema, error) {
	as := &abiSchema{
		Schema: persisted,
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

func (as *abiSchema) Type() pldapi.SchemaType {
	return pldapi.SchemaTypeABI
}

func (as *abiSchema) ID() pldtypes.Bytes32 {
	return as.Schema.ID
}

func (as *abiSchema) Signature() string {
	return as.Schema.Signature
}

func (as *abiSchema) Persisted() *pldapi.Schema {
	return as.Schema
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

func (as *abiSchema) buildLabel(ctx context.Context, fieldName string, f *abi.ComponentValue) (*pldapi.StateLabel, *pldapi.StateInt64Label, error) {
	labelType, err := as.getLabelType(ctx, fieldName, f.Component)
	if err != nil {
		return nil, nil, err
	}
	return as.mapValueToLabel(ctx, fieldName, labelType, f)
}

func (as *abiSchema) mapValueToLabel(ctx context.Context, fieldName string, labelType labelType, f *abi.ComponentValue) (*pldapi.StateLabel, *pldapi.StateInt64Label, error) {
	switch labelType {
	case labelTypeInt64:
		bigIntVal, ok := f.Value.(*big.Int)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, new(big.Int))
		}
		return nil, &pldapi.StateInt64Label{Label: fieldName, Value: bigIntVal.Int64()}, nil
	case labelTypeInt256:
		bigIntVal, ok := f.Value.(*big.Int)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, new(big.Int))
		}
		// Otherwise we fall back to encoding as a fixed-width hex string - with a leading sign character
		filterString := pldtypes.Int256To65CharDBSafeSortableString(bigIntVal)
		return &pldapi.StateLabel{Label: fieldName, Value: filterString}, nil, nil
	case labelTypeUint256:
		bigIntVal, ok := f.Value.(*big.Int)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, new(big.Int))
		}
		bigIntVal = bigIntVal.Abs(bigIntVal)
		filterString := filters.Uint256ToFilterString(ctx, bigIntVal)
		return &pldapi.StateLabel{Label: fieldName, Value: filterString}, nil, nil
	case labelTypeBytes:
		byteValue, ok := f.Value.([]byte)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, []byte{})
		}
		// We do NOT use an 0x prefix on bytes types
		return &pldapi.StateLabel{Label: fieldName, Value: hex.EncodeToString(byteValue)}, nil, nil
	case labelTypeString:
		strValue, ok := f.Value.(string)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, "")
		}
		return &pldapi.StateLabel{Label: fieldName, Value: strValue}, nil, nil
	case labelTypeBool:
		bValue, ok := f.Value.(*big.Int)
		if !ok {
			return nil, nil, i18n.NewError(ctx, msgs.MsgStateLabelFieldUnexpectedValue, fieldName, f.Value, new(big.Int))
		}
		return nil, &pldapi.StateInt64Label{Label: fieldName, Value: bValue.Int64()}, nil
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
	labels      []*pldapi.StateLabel
	int64Labels []*pldapi.StateInt64Label
	labelValues filters.PassthroughValueSet
}

func (as *abiSchema) parseStateData(ctx context.Context, data pldtypes.RawJSON) (*parsedStateData, error) {
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
func (as *abiSchema) ProcessState(ctx context.Context, contractAddress *pldtypes.EthAddress, data pldtypes.RawJSON, id pldtypes.HexBytes, customHashFunction bool) (*components.StateWithLabels, error) {

	// We need to re-serialize the data according to the ABI to:
	// - Ensure it's valid
	// - Remove anything that is not part of the schema
	// - Standardize formatting of all the data elements so domains do not need to worry
	var jsonData []byte
	psd, err := as.parseStateData(ctx, data)
	if err == nil {
		jsonData, err = pldtypes.StandardABISerializer().SerializeJSONCtx(ctx, psd.cv)
	}
	if err != nil {
		return nil, err
	}

	// The ID must be unique, but domains can choose whether they calculate it or they defer to Paladin
	// to calculate it. Domains (ZKP based domains in particular) that need specific algorithms to be used for
	// hash calculation, or have other requirements of the derivation of the hash can pre-calculate the hash.
	//
	// Implementations MUST ensure:
	// - The hash contains everything in the state that needs to be proved
	// - The hash is deterministic and reproducible by anyone with access to the unmasked state data
	//
	// Note this function only validates Paladin-default hashes, when customHashFunction is true
	// the caller must have pre-verified the hash
	if customHashFunction {
		if id == nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgStateIDMissing)
		}
	} else {
		// When Paladin is designated to create that hash, it uses a EIP-712 Typed Data V4 hash as this has
		// the characteristics of:
		// - Well proven and Ethereum standardized algorithm for hashing a complex structure
		// - Deterministic order and type formatting of values
		// - Only containing the data that is described in the associated the ABI
		var hash ethtypes.HexBytes0xPrefix
		hash, err = eip712.HashStruct(ctx, as.primaryType, psd.jsonTree, as.typeSet)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgStateInvalidCalculatingHash)
		}
		if id != nil && !id.Equals(pldtypes.HexBytes(hash)) {
			return nil, i18n.NewError(ctx, msgs.MsgStateHashMismatch, id, hash)
		}
		id = pldtypes.HexBytes(hash)
	}

	for i := range psd.labels {
		psd.labels[i].DomainName = as.DomainName
		psd.labels[i].State = id
	}
	for i := range psd.int64Labels {
		psd.int64Labels[i].DomainName = as.DomainName
		psd.int64Labels[i].State = id
	}

	now := pldtypes.TimestampNow()
	return &components.StateWithLabels{
		State: &pldapi.State{
			StateBase: pldapi.StateBase{
				ID:              id,
				Created:         now,
				DomainName:      as.DomainName,
				Schema:          as.Schema.ID,
				ContractAddress: contractAddress,
				Data:            jsonData,
			},
			Labels:      psd.labels,
			Int64Labels: psd.int64Labels,
		},
		LabelValues: addStateBaseLabels(psd.labelValues, id, now),
	}, nil
}

func (as *abiSchema) RecoverLabels(ctx context.Context, s *pldapi.State) (*components.StateWithLabels, error) {
	psd, err := as.parseStateData(ctx, s.Data)
	if err != nil {
		return nil, err
	}
	return &components.StateWithLabels{
		State:       s,
		LabelValues: addStateBaseLabels(psd.labelValues, s.ID, s.Created),
	}, nil
}
