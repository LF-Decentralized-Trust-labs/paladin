// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/pldmsgs"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"k8s.io/utils/ptr"
)

type TypeReferenceDoc struct {
	Example           []byte
	Description       []byte
	FieldDescriptions []byte
	SubFieldTables    []byte
}

type MethodReferenceDoc struct {
	Example     []byte
	Description []byte
	Params      []byte
	Return      []byte
}

// MethodDescription holds metadata about a method, including inputs and outputs.
type methodDescription struct {
	name    string
	inputs  []param
	outputs []param
}

// Param holds information about a method parameter or return type.
type param struct {
	name      string
	typeName  string
	isList    bool
	isPointer bool
}

/*
 * This function generates a series of markdown pages to document Paladin types, and are
 * designed to be included in the docs. Each page is a []byte value in the map, and the
 * key is the file name of the page. To add additional pages, simply create an example
 * instance of the type you would like to document, then include that in the `types`
 * array which is passed to generateMarkdownPages(). Note: It is the responsibility of
 * some other caller function to actually write the bytes to disk.
 */

var allTypes = []interface{}{
	pldapi.IndexedEvent{},
	pldapi.TransactionReceipt{},
	pldapi.TransactionReceiptFull{},
	pldapi.TransactionReceiptListener{},
	pldapi.TransactionReceiptFilters{},
	pldapi.TransactionReceiptListenerOptions{},
	pldapi.TransactionStates{},
	pldapi.TransactionInput{},
	pldapi.TransactionFull{},
	pldapi.TransactionCall{},
	pldapi.Transaction{},
	pldapi.PreparedTransaction{},
	pldapi.PublicTx{},
	pldapi.StoredABI{
		ABI: abi.ABI{
			&abi.Entry{
				Type:            "function",
				Name:            "name",
				StateMutability: "pure",
			},
		},
		Hash: pldtypes.Bytes32{},
	},
	pldapi.State{},
	pldapi.StateConfirmRecord{},
	pldapi.StateSpendRecord{},
	pldapi.StateLock{},
	pldapi.Schema{},
	pldapi.RegistryEntry{OnChainLocation: &pldapi.OnChainLocation{}},
	pldapi.RegistryEntryWithProperties{
		RegistryEntry: &pldapi.RegistryEntry{
			OnChainLocation: &pldapi.OnChainLocation{},
		},
	},
	pldapi.RegistryProperty{},
	pldapi.OnChainLocation{},
	pldapi.IndexedBlock{},
	pldapi.IndexedTransaction{},
	pldapi.IndexedEvent{},
	pldapi.EventWithData{},
	pldapi.ABIDecodedData{},
	pldapi.PeerInfo{},
	pldapi.KeyMappingAndVerifier{},
	pldapi.ReliableMessageAck{},
	pldapi.ReliableMessage{},
	pldapi.PrivacyGroup{},
	pldapi.PrivacyGroupEVMCall{},
	pldapi.PrivacyGroupEVMTXInput{},
	pldapi.PrivacyGroupInput{},
	pldapi.PrivacyGroupMessageListener{},
	pldapi.PrivacyGroupMessage{},
	pldapi.PrivacyGroupMessageInput{},
	pldtypes.JSONFormatOptions(""),
	pldapi.StateStatusQualifier(""),
	query.QueryJSON{
		Limit: ptr.To(10),
		Sort:  []string{"field1 DESC", "field2"},
		Statements: query.Statements{
			Ops: query.Ops{
				Eq: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "field1",
						},
						Value: pldtypes.RawJSON(`"abcde"`),
					},
					{
						Op: query.Op{
							Field:           "field12",
							Not:             true,
							CaseInsensitive: true,
						},
						Value: pldtypes.RawJSON(`"abcde"`),
					},
				},
				NEq: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "field2",
						},
						Value: pldtypes.RawJSON(`"abcde"`),
					},
				},
				Like: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "field3",
						},
						Value: pldtypes.RawJSON(`"abcde"`),
					},
				},
				LT: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "field4",
						},
						Value: pldtypes.RawJSON([]byte(`12345`)),
					},
				},
				LTE: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "field5",
						},
						Value: pldtypes.RawJSON([]byte(`12345`)),
					},
				},
				GT: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "field6",
						},
						Value: pldtypes.RawJSON([]byte(`12345`)),
					},
				},
				GTE: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "field7",
						},
						Value: pldtypes.RawJSON([]byte(`12345`)),
					},
				},
				In: []*query.OpMultiVal{
					{
						Op: query.Op{
							Field: "field8",
						},
						Values: []pldtypes.RawJSON{[]byte(`"abcde"`), []byte(`"fghij"`)},
					},
				},
				NIn: []*query.OpMultiVal{
					{
						Op: query.Op{
							Field: "field9",
						},
						Values: []pldtypes.RawJSON{[]byte(`"abcde"`), []byte(`"fghij"`)},
					},
				},
				Null: []*query.Op{
					{
						Field: "field1",
						Not:   true,
					},
					{
						Field: "field2",
					},
				},
			},
		},
	},
	pldapi.BlockchainEventListener{},
	pldapi.BlockchainEventListenerOptions{},
	pldapi.BlockchainEventListenerSource{},
	pldapi.BlockchainEventListenerStatus{},
	pldapi.BlockchainEventListenerCheckpoint{},
}
var allAPITypes = []pldclient.RPCModule{
	pldclient.New().PTX(),
	pldclient.New().KeyManager(),
	pldclient.New().Registry(),
	pldclient.New().Transport(),
	pldclient.New().StateStore(),
	pldclient.New().BlockIndex(),
	pldclient.New().PrivacyGroups(),
}

var allSimpleTypes = []interface{}{
	pldtypes.Bytes32{},
	pldtypes.HexBytes{},
	pldtypes.EthAddress{},
	pldtypes.HexUint256{},
	pldtypes.HexInt256{},
	uuid.UUID{},
	pldtypes.HexUint64OrString(""),
	pldtypes.HexUint64(0),
	pldtypes.Timestamp(0),
	pldtypes.RawJSON([]byte{}),
	pldtypes.PrivateIdentityLocator(""),
}

type docGenerator struct {
	pageToTypes map[string][]string
	typeToPage  map[string]string
}

func newDocGenerator() *docGenerator {
	return &docGenerator{
		pageToTypes: make(map[string][]string),
		typeToPage:  make(map[string]string),
	}
}

func GenerateObjectsReferenceMarkdown(ctx context.Context) (map[string][]byte, error) {
	d := newDocGenerator()

	types := allTypes
	simpleTypes := allSimpleTypes
	apiTypes := allAPITypes

	return d.generateMarkdownPages(ctx, types, simpleTypes, apiTypes, getRelativePath(4))
}

func (d *docGenerator) generateMarkdownPages(ctx context.Context, types, simpleTypes []interface{}, apiTypes []pldclient.RPCModule, outputPath string) (map[string][]byte, error) {
	markdownMap := make(map[string][]byte, len(types)+len(apiTypes)+1)

	typesPath := filepath.Join(outputPath, "types")
	apisPath := filepath.Join(outputPath, "apis")

	pageName := "simpletypes"
	markdownMap[filepath.Join(typesPath, pageName+".md")] = d.generateSimpleTypesMarkdown(ctx, simpleTypes, pageName, typesPath)

	// first add all pages to map
	for i := range types {
		pageTitle := getType(types[i]).Name()
		d.pageToTypes[strings.ToLower(pageTitle)] = []string{}
	}

	// have to go round twice to ensure we cross-link correctly. First to add them to the map
	for i, o := range types {
		fmt.Println(getType(types[i]).Name())
		pageTitle := getType(types[i]).Name()
		pageName := strings.ToLower(pageTitle)
		d.addPageToMap(reflect.TypeOf(o), pageName)
	}

	// ... then to build the content
	for i, o := range types {
		pageTitle := getType(types[i]).Name()
		pageName := strings.ToLower(pageTitle)
		// Page index starts at 1. Simple types will be the first page. Everything else comes after that.
		pageHeader := generatePageHeader(pageTitle)
		b := bytes.NewBuffer([]byte(pageHeader))
		markdown, err := d.generateObjectReferenceMarkdown(ctx, true, o, reflect.TypeOf(o), pageName, typesPath)
		if err != nil {
			return nil, err
		}
		b.Write(markdown)
		markdownMap[filepath.Join(typesPath, pageName+".md")] = b.Bytes()
	}

	// add api types
	for _, apiGroup := range apiTypes {
		pageTitle := fmt.Sprintf("%s_*", apiGroup.Group())
		pageName := strings.ToLower(apiGroup.Group())

		// Page index starts at 1. Simple types will be the first page. Everything else comes after that.
		pageHeader := generatePageHeader(pageTitle)
		b := bytes.NewBuffer([]byte(pageHeader))
		markdown, err := d.generateMethodTypesMarkdown(ctx, apiGroup, apisPath)
		if err != nil {
			return nil, err
		}
		b.Write(markdown)

		markdownMap[filepath.Join(apisPath, pageName+".md")] = b.Bytes()
	}

	return markdownMap, nil
}

func (d *docGenerator) generateMethodTypesMarkdown(ctx context.Context, apiGroup pldclient.RPCModule, outputPath string) ([]byte, error) {
	apiGroupType := reflect.TypeOf(apiGroup)
	reflectMethods := make(map[string]reflect.Method)
	for _, methodName := range apiGroup.Methods() {
		groupPrefix := apiGroup.Group() + "_"
		requiredFnName, hasPrefix := strings.CutPrefix(methodName, groupPrefix)
		if !hasPrefix {
			return nil, fmt.Errorf("RPC method '%s' does not start with prefix %s", requiredFnName, groupPrefix)
		}
		requiredFnName = strings.ToUpper(requiredFnName[0:1]) + requiredFnName[1:]

		var method *reflect.Method
		for i := 0; i < apiGroupType.NumMethod(); i++ {
			m := apiGroupType.Method(i)
			if m.Name == requiredFnName {
				method = &m
			}
		}
		if method == nil {
			return nil, fmt.Errorf("Implementation method '%s' for RPC method '%s' does not exist on interface %T", requiredFnName, methodName, apiGroup)
		}
		reflectMethods[methodName] = *method
	}

	methods, err := d.generateMethodDescriptions(apiGroup, reflectMethods)
	if err != nil {
		return nil, err
	}

	b := bytes.NewBuffer([]byte{})
	for _, method := range methods {
		b.WriteString(fmt.Sprintf("## `%s`\n\n", method.name))
		markdown, _ := d.generateMethodReferenceMarkdown(ctx, method, outputPath)
		b.Write(markdown)
	}
	return b.Bytes(), nil
}

func (d *docGenerator) generateMethodReferenceMarkdown(ctx context.Context, method methodDescription, outputPath string) ([]byte, error) {
	methodReferenceDoc := MethodReferenceDoc{}

	des, err := getIncludeFile(ctx, outputPath, strings.ToLower(method.name))
	if err != nil {
		fmt.Printf("missing description for method %s\n", method.name)
	}
	methodReferenceDoc.Description = des

	// TODO: Add example???
	// typeReferenceDoc.Example = exampleJSON

	// buff is the main buffer where we will write the markdown for this page
	buff := bytes.NewBuffer([]byte{})

	if methodReferenceDoc.Description != nil {
		buff.Write(methodReferenceDoc.Description)
	}

	// **Add Parameters Section**
	if len(method.inputs) > 0 {
		buff.WriteString("### Parameters\n\n")
		for i, param := range method.inputs {
			buff.WriteString(fmt.Sprintf("%d. %s\n", i, d.getParamField(param)))
		}
		buff.WriteString("\n")
	}

	// **Add Return Section**
	if len(method.outputs) > 0 {
		buff.WriteString("### Returns\n\n")
		for i, param := range method.outputs {
			buff.WriteString(fmt.Sprintf("%d. %s\n", i, d.getParamField(param)))
		}
		buff.WriteString("\n")
	}

	// **Add Example Section if Available**
	if len(methodReferenceDoc.Example) > 0 {
		buff.WriteString("### Example\n\n```json\n")
		buff.Write(methodReferenceDoc.Example)
		buff.WriteString("\n```\n\n")
	}

	return buff.Bytes(), nil
}

func (d *docGenerator) getParamField(p param) string {
	listMarker := ""
	if p.isList {
		listMarker = "[]"
	}
	pldType := fmt.Sprintf("`%s%s`", p.typeName, listMarker)
	link := ""
	if fileName, ok := d.typeToPage[strings.ToLower(p.typeName)]; ok {
		link = fmt.Sprintf("../types/%s.md#%s", fileName, strings.ToLower(p.typeName))
	}
	if link != "" {
		pldType = fmt.Sprintf("[%s](%s)", pldType, link)
	}
	return fmt.Sprintf("`%s`: %s", p.name, pldType)
}

// generateMethodDescriptions extracts metadata about all methods of a struct/interface.
func (d *docGenerator) generateMethodDescriptions(apiGroup pldclient.RPCModule, reflectMethods map[string]reflect.Method) (_ []methodDescription, err error) {
	var methods []methodDescription

	// Iterate over all methods.
	for _, methodName := range apiGroup.Methods() {
		reflectMethod := reflectMethods[methodName]
		methodDesc := methodDescription{name: methodName}
		methodDesc.inputs, err = d.extractParams(reflectMethod.Type, apiGroup.MethodInfo(methodName), true)
		if err == nil {
			methodDesc.outputs, err = d.extractParams(reflectMethod.Type, apiGroup.MethodInfo(methodName), false)
		}
		if err != nil {
			return nil, err
		}
		methods = append(methods, methodDesc)
	}
	return methods, err
}

// extractParams extracts input or output parameters from a method type.
func (d *docGenerator) extractParams(funcType reflect.Type, methodInfo *pldclient.RPCMethodInfo, isInput bool) ([]param, error) {
	var params []param
	var count int
	discardStart := 0
	discardEnd := 0

	if isInput {
		count = funcType.NumIn()
		discardStart = 2 // discard the struct pointer, and the context variable
		discardEnd = 0
	} else {
		count = funcType.NumOut()
		discardStart = 0
		discardEnd = 1 //discard the error parameters
	}
	docCount := count - discardStart - discardEnd

	var nameArray []string
	if isInput {
		nameArray = methodInfo.Inputs
	} else {
		nameArray = []string{methodInfo.Output}
	}
	if len(nameArray) != docCount {
		return nil, fmt.Errorf("function %s has %d inputs/outputs (inputs=%t), but list is %v", funcType.Name(), docCount, isInput, nameArray)
	}

	// Iterate over parameters.
	for j := discardStart; j < count-discardEnd; j++ {
		var paramType reflect.Type
		if isInput {
			paramType = funcType.In(j)
		} else {
			paramType = funcType.Out(j)
		}

		// Use global filter logic to skip unwanted types.
		if shouldFilter(paramType) {
			continue
		}

		// First check if this is a named type we know of directly
		_, isKnown := d.typeToPage[strings.ToLower(paramType.Name())]
		isList := false
		isPointer := false
		if !isKnown {

			// FIXME: I'm not sure a pointer to a slice will parse correctly (because currently we first handle the slice and then the pointer)
			isList = paramType.Kind() == reflect.Slice
			if isList {
				paramType = paramType.Elem()
			}

			isPointer = paramType.Kind() == reflect.Ptr
			if isPointer {
				paramType = paramType.Elem()
			}

		}

		typeName := paramType.Name()
		if isEnum(paramType) {
			typeName = generateEnumList(paramType)
		} else if paramType.Kind() == reflect.Struct {
			if _, ok := d.typeToPage[strings.ToLower(typeName)]; !ok {
				panic(fmt.Sprintf("Missing documentation example for '%s' - add to allTypes", typeName))
			}
		}

		// Store the parameter metadata.
		param := param{
			name:      nameArray[j-discardStart],
			typeName:  typeName,
			isList:    isList,
			isPointer: isPointer,
		}
		params = append(params, param)
	}
	return params, nil
}

func (d *docGenerator) generateSimpleTypesMarkdown(ctx context.Context, simpleTypes []interface{}, pageName, outputPath string) []byte {

	pageHeader := generatePageHeader("Simple Types")
	b := bytes.NewBuffer([]byte(pageHeader))
	for _, simpleType := range simpleTypes {
		b.WriteString(fmt.Sprintf("## %s\n\n", reflect.TypeOf(simpleType).Name()))
		d.addPageToMap(reflect.TypeOf(simpleType), pageName)
		markdown, _ := d.generateObjectReferenceMarkdown(ctx, true, nil, reflect.TypeOf(simpleType), pageName, outputPath)
		b.Write(markdown)
	}
	return b.Bytes()
}

func (d *docGenerator) addPageToMap(t reflect.Type, pageName string) {
	// typeToPage is where we keep track of all the tables we've generated (recursively)
	// for creating hyperlinks within the markdown
	d.typeToPage[strings.ToLower(t.Name())] = pageName
	d.pageToTypes[pageName] = append(d.pageToTypes[pageName], t.Name())
}

func (d *docGenerator) generateObjectReferenceMarkdown(ctx context.Context, descRequired bool, example interface{}, t reflect.Type, pageName, outputPath string) ([]byte, error) {
	typeReferenceDoc := TypeReferenceDoc{}

	if t.Kind() == reflect.Ptr {
		t = reflect.TypeOf(example).Elem()
	}

	des, err := getIncludeFile(ctx, outputPath, strings.ToLower(t.Name()))
	if descRequired && err != nil {
		return nil, err
	}
	typeReferenceDoc.Description = des

	// Include an example JSON representation if we have one available
	if example != nil {
		exampleJSON, err := json.MarshalIndent(example, "", "    ")
		if err != nil {
			return nil, err
		}
		typeReferenceDoc.Example = exampleJSON
	}

	// If the type is a struct, look into each field inside it
	if t.Kind() == reflect.Struct {
		typeReferenceDoc.FieldDescriptions, typeReferenceDoc.SubFieldTables, err = d.generateFieldDescriptionsForStruct(ctx, t, pageName, outputPath)
		if err != nil {
			return nil, err
		}
	}

	// buff is the main buffer where we will write the markdown for this page
	buff := bytes.NewBuffer([]byte{})

	// If we only have one section, we will not write H3 headers
	sectionCount := 0
	if typeReferenceDoc.Description != nil {
		sectionCount++
	}
	if typeReferenceDoc.Example != nil {
		sectionCount++
	}
	if typeReferenceDoc.FieldDescriptions != nil {
		sectionCount++
	}

	if typeReferenceDoc.Description != nil {
		buff.Write(typeReferenceDoc.Description)
	}
	if len(typeReferenceDoc.Example) > 0 {
		if sectionCount > 1 {
			buff.WriteString("### Example\n\n```json\n")
		}
		buff.Write(typeReferenceDoc.Example)
		buff.WriteString("\n```\n\n")
	}
	if len(typeReferenceDoc.FieldDescriptions) > 0 {
		if sectionCount > 1 {
			buff.WriteString("### Field Descriptions\n\n")
		}
		buff.Write(typeReferenceDoc.FieldDescriptions)
		buff.WriteString("\n")
	}

	if len(typeReferenceDoc.SubFieldTables) > 0 {
		buff.Write(typeReferenceDoc.SubFieldTables)
	}

	return buff.Bytes(), nil
}

func (d *docGenerator) generateFieldDescriptionsForStruct(ctx context.Context, t reflect.Type, pageName string, outputPath string) ([]byte, []byte, error) {
	var err error

	fieldDescriptionsBytes := []byte{}
	// subFieldBuff is where we write any additional tables for sub fields that may be on this struct
	subFieldBuff := bytes.NewBuffer([]byte{})
	if t.NumField() > 0 {
		// Write the table to a temporary buffer - we will throw it away if there are no
		// public JSON serializable fields on the struct
		tableRowCount := 0
		tableBuff := bytes.NewBuffer([]byte{})
		tableBuff.WriteString("| Field Name | Description | Type |\n")
		tableBuff.WriteString("|------------|-------------|------|\n")
		tableRowCount, err = d.writeStructFields(ctx, t, pageName, outputPath, subFieldBuff, tableBuff, tableRowCount)
		if err != nil {
			return nil, nil, err
		}
		if tableRowCount > 0 {
			fieldDescriptionsBytes = tableBuff.Bytes()
		}
	}

	return fieldDescriptionsBytes, subFieldBuff.Bytes(), err
}

func (d *docGenerator) writeStructFields(ctx context.Context, t reflect.Type, pageName string, outputPath string, subFieldBuff, tableBuff *bytes.Buffer, tableRowCount int) (int, error) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		structTag := field.Tag.Get("docstruct")
		excludeTag := field.Tag.Get("docexclude")

		// If this is a nested struct, we need to recurse into it
		if field.Anonymous {
			structType := field.Type
			if structType.Kind() == reflect.Pointer {
				structType = structType.Elem()
			}
			var err error
			tableRowCount, err = d.writeStructFields(ctx, structType, pageName, outputPath, subFieldBuff, tableBuff, tableRowCount)
			if err != nil {
				return tableRowCount, err
			}
			continue
		}

		// If the field is specifically excluded, or doesn't have a json tag, skip it
		if excludeTag != "" || jsonTag == "" || jsonTag == "-" {
			continue
		}

		jsonFieldName := strings.Split(jsonTag, ",")[0]
		messageKeyName := fmt.Sprintf("%s.%s", structTag, jsonFieldName)
		description := i18n.Expand(ctx, i18n.MessageKey(messageKeyName))
		if description == messageKeyName {
			return tableRowCount, i18n.NewError(ctx, pldmsgs.MsgFieldDescriptionMissing, jsonFieldName, t.Name())
		}

		isArray := false

		fieldType := field.Type
		pldType := fieldType.Name()

		_, isKnown := d.typeToPage[strings.ToLower(pldType)]
		if pldType == "" /* a plain slice rather than named one */ || !isKnown {
			if fieldType.Kind() == reflect.Slice {
				fieldType = fieldType.Elem()
				pldType = fieldType.Name()
				isArray = true
			}

			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
				pldType = fieldType.Name()
			}
		}
		if isArray {
			pldType = fmt.Sprintf("%s[]", pldType)
		}

		pldType = fmt.Sprintf("`%s`", pldType)

		isStruct := fieldType.Kind() == reflect.Struct

		link := ""
		if isEnum(field.Type) {
			pldType = fmt.Sprintf("`%s`", generateEnumList(field.Type))
		} else if p, ok := d.typeToPage[strings.ToLower(fieldType.Name())]; ok && p != pageName {
			link = fmt.Sprintf("%s.md#%s", p, strings.ToLower(fieldType.Name()))
		} else if isStruct {
			link = fmt.Sprintf("#%s", strings.ToLower(fieldType.Name()))
		}

		if link != "" {
			pldType = fmt.Sprintf("[%s](%s)", pldType, link)

			// Generate the table for the sub type
			_, typeAlreadyGenerated := d.typeToPage[strings.ToLower(fieldType.Name())]
			_, page := d.pageToTypes[strings.ToLower(fieldType.Name())]

			if isStruct && !typeAlreadyGenerated && !page {
				d.addPageToMap(fieldType, pageName)

				subFieldBuff.WriteString(fmt.Sprintf("## %s\n\n", fieldType.Name()))
				subFieldMarkdown, _ := d.generateObjectReferenceMarkdown(ctx, false, nil, fieldType, pageName, outputPath)
				subFieldBuff.Write(subFieldMarkdown)
				subFieldBuff.WriteString("\n")
			}
		}

		tableBuff.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", jsonFieldName, description, pldType))
		tableRowCount++
	}
	return tableRowCount, nil
}

func generatePageHeader(pageTitle string) string {
	return fmt.Sprintf(`---
title: %s
---
`, pageTitle)
}
