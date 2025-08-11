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

package ethclient

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type ABIFunctionClient interface {
	ABI() abi.ABI
	ABIEntry() *abi.Entry
	R(ctx context.Context) ABIFunctionRequestBuilder
}

type EthTXVersion string

const (
	LEGACY_ORIGINAL EthTXVersion = "legacy_original"
	LEGACY_EIP155   EthTXVersion = "legacy_eip155"
	EIP1559         EthTXVersion = "eip1559"
)

type ABIClient interface {
	ABI() abi.ABI
	Function(ctx context.Context, nameOrFullSig string) (_ ABIFunctionClient, err error)
	MustFunction(nameOrFullSig string) ABIFunctionClient
	Constructor(ctx context.Context, bytecode pldtypes.HexBytes) (_ ABIFunctionClient, err error)
	MustConstructor(bytecode pldtypes.HexBytes) ABIFunctionClient
}

type ABIFunctionRequestBuilder interface {
	// Builder functions
	TXVersion(EthTXVersion) ABIFunctionRequestBuilder
	Signer(string) ABIFunctionRequestBuilder
	To(*ethtypes.Address0xHex) ABIFunctionRequestBuilder
	GasLimit(uint64) ABIFunctionRequestBuilder
	BlockRef(blockRef BlockRef) ABIFunctionRequestBuilder
	Block(uint64) ABIFunctionRequestBuilder
	Input(any) ABIFunctionRequestBuilder
	Output(any) ABIFunctionRequestBuilder
	Serializer(*abi.Serializer) ABIFunctionRequestBuilder
	CallOptions(...CallOption) ABIFunctionRequestBuilder // adds extra call options (defaults are to use the ABI for errors and decoding)

	// Query functions
	TX() *ethsigner.Transaction

	// Execution functions
	BuildCallData() (err error)              // finalizes the call data in the TX(), but does not perform any JSON/RPC calls
	Call() (err error)                       // calls and processes the result back into the output struct supplied in the builder
	CallResult() (res CallResult, err error) // returns the detailed result - parsing the response against the ABI, but not re-marshaling it into your object
	EstimateGas() (res EstimateGasResult, err error)
	RawTransaction() (rawTX pldtypes.HexBytes, err error)
	SignAndSend() (txHash *pldtypes.Bytes32, err error)
}

type BlockRef string

const (
	LATEST    BlockRef = "latest"
	EARLIEST  BlockRef = "earliest"
	PENDING   BlockRef = "pending"
	SAFE      BlockRef = "safe"
	FINALIZED BlockRef = "finalized"
)

type abiClient struct {
	ec        *ethClient
	abi       abi.ABI
	functions map[string]*abi.Entry
}

type abiFunctionClient struct {
	ec          *ethClient
	bytecode    pldtypes.HexBytes
	signature   string
	selector    []byte
	abi         abi.ABI
	abiEntry    *abi.Entry
	inputCount  int
	inputs      abi.TypeComponent
	outputCount int
	outputs     abi.TypeComponent
	serializer  *abi.Serializer
}

type abiFunctionRequestBuilder struct {
	*abiFunctionClient
	ctx          context.Context
	txVersion    EthTXVersion
	tx           ethsigner.Transaction
	block        string
	fromStr      *string
	input        any
	output       any
	extendedOpts []CallOption
}

func (ec *ethClient) ABIJSON(ctx context.Context, abiJson []byte) (ABIClient, error) {
	var a abi.ABI
	err := json.Unmarshal(abiJson, &a)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgEthClientABIJson)
	}
	return ec.ABI(ctx, a)
}

func (ec *ethClient) ABI(ctx context.Context, a abi.ABI) (ABIClient, error) {
	functions := map[string]*abi.Entry{}
	for _, e := range a {
		s, err := e.SignatureCtx(ctx)
		if err != nil {
			return nil, err
		}
		if e.Name != "" && e.IsFunction() {
			for i, o := range e.Outputs {
				if o.Name == "" {
					o.Name = strconv.Itoa(i)
				}
			}
			functions[e.Name] = e
			functions[s] = e
		}
	}
	return &abiClient{
		ec:        ec,
		abi:       a,
		functions: functions,
	}, nil
}

func (ec *ethClient) MustABIJSON(abiJson []byte) ABIClient {
	abic, err := ec.ABIJSON(context.Background(), abiJson)
	if err != nil {
		panic(err)
	}
	return abic
}

func (abic *abiClient) Function(ctx context.Context, nameOrFullSig string) (_ ABIFunctionClient, err error) {
	ac := &abiFunctionClient{ec: abic.ec, abi: abic.abi}
	functionABI := abic.functions[nameOrFullSig]
	if functionABI == nil {
		err = i18n.NewError(ctx, msgs.MsgEthClientFunctionNotFound, nameOrFullSig)
	}
	if err == nil {
		ac.selector, err = functionABI.GenerateFunctionSelectorCtx(ctx)
	}
	if err != nil {
		return nil, err
	}
	return ac.functionCommon(ctx, functionABI)
}

func (ec *ethClient) ABIFunction(ctx context.Context, functionABI *abi.Entry) (fc ABIFunctionClient, err error) {
	a, err := ec.ABI(ctx, abi.ABI{functionABI})
	if err == nil {
		fc, err = a.Function(ctx, functionABI.Name)
	}
	return fc, err
}

func (abic *abiClient) Constructor(ctx context.Context, bytecode pldtypes.HexBytes) (ABIFunctionClient, error) {
	ac := &abiFunctionClient{ec: abic.ec, bytecode: bytecode, abi: abic.abi}
	functionABI := abic.abi.Constructor()
	if functionABI == nil {
		// Default constructor
		functionABI = &abi.Entry{
			Type:    abi.Constructor,
			Inputs:  abi.ParameterArray{},
			Outputs: abi.ParameterArray{},
		}
	}
	return ac.functionCommon(ctx, functionABI)
}

func (ec *ethClient) ABIConstructor(ctx context.Context, constructorABI *abi.Entry, bytecode pldtypes.HexBytes) (fc ABIFunctionClient, err error) {
	a, err := ec.ABI(ctx, abi.ABI{constructorABI})
	if err == nil {
		fc, err = a.Constructor(ctx, bytecode)
	}
	return fc, err
}

func (ac *abiFunctionClient) functionCommon(ctx context.Context, functionABI *abi.Entry) (_ ABIFunctionClient, err error) {
	ac.abiEntry = functionABI
	ac.signature, err = functionABI.SignatureCtx(ctx)
	if err == nil {
		ac.inputCount = len(functionABI.Inputs)
		ac.inputs, err = functionABI.Inputs.TypeComponentTreeCtx(ctx)
	}
	if err == nil {
		ac.outputCount = len(functionABI.Outputs)
		ac.outputs, err = functionABI.Outputs.TypeComponentTreeCtx(ctx)
	}
	if err != nil {
		return nil, err
	}
	return ac, nil
}

func (abic *abiClient) MustFunction(nameOrFullSig string) ABIFunctionClient {
	ac, err := abic.Function(context.Background(), nameOrFullSig)
	if err != nil {
		panic(err)
	}
	return ac
}

func (abic *abiClient) MustConstructor(bytecode pldtypes.HexBytes) ABIFunctionClient {
	ac, err := abic.Constructor(context.Background(), bytecode)
	if err != nil {
		panic(err)
	}
	return ac
}

func (abic *abiClient) ABI() abi.ABI {
	return abic.abi
}

func (ac *abiFunctionClient) ABI() abi.ABI {
	return ac.abi
}

func (ac *abiFunctionClient) ABIEntry() *abi.Entry {
	return ac.abiEntry
}

func (ac *abiFunctionClient) R(ctx context.Context) ABIFunctionRequestBuilder {
	return &abiFunctionRequestBuilder{
		ctx:               ctx,
		txVersion:         EIP1559,
		abiFunctionClient: ac,
		block:             "latest",
	}
}

func (ac *abiFunctionRequestBuilder) TXVersion(v EthTXVersion) ABIFunctionRequestBuilder {
	ac.txVersion = v
	return ac
}

func (ac *abiFunctionRequestBuilder) Signer(fromStr string) ABIFunctionRequestBuilder {
	ac.fromStr = &fromStr
	return ac
}

func (ac *abiFunctionRequestBuilder) To(to *ethtypes.Address0xHex) ABIFunctionRequestBuilder {
	ac.tx.To = to
	return ac
}

func (ac *abiFunctionRequestBuilder) GasLimit(gasLimit uint64) ABIFunctionRequestBuilder {
	ac.tx.GasLimit = ethtypes.NewHexIntegerU64(gasLimit)
	return ac
}

func (ac *abiFunctionRequestBuilder) BlockRef(blockRef BlockRef) ABIFunctionRequestBuilder {
	ac.block = string(blockRef)
	return ac
}

func (ac *abiFunctionRequestBuilder) Block(block uint64) ABIFunctionRequestBuilder {
	ac.block = "0x" + strconv.FormatUint(block, 16)
	return ac
}

func (ac *abiFunctionRequestBuilder) Input(input any) ABIFunctionRequestBuilder {
	ac.input = input
	return ac
}

func (ac *abiFunctionRequestBuilder) Output(output any) ABIFunctionRequestBuilder {
	ac.output = output
	return ac
}

func (ac *abiFunctionRequestBuilder) CallOptions(extendedOpts ...CallOption) ABIFunctionRequestBuilder {
	ac.extendedOpts = append(ac.extendedOpts, extendedOpts...)
	return ac
}

func (ac *abiFunctionRequestBuilder) Serializer(serializer *abi.Serializer) ABIFunctionRequestBuilder {
	ac.serializer = serializer
	return ac
}

func (ac *abiFunctionRequestBuilder) TX() *ethsigner.Transaction {
	return &ac.tx
}

func (ac *abiFunctionRequestBuilder) validateTo() error {
	if ac.tx.To != nil && ac.selector == nil {
		return i18n.NewError(ac.ctx, msgs.MsgEthClientToWithConstructor)
	} else if ac.tx.To == nil && ac.selector != nil {
		return i18n.NewError(ac.ctx, msgs.MsgEthClientMissingTo)
	}
	return nil
}

func (ac *abiFunctionRequestBuilder) BuildCallData() (err error) {
	// Encode the call data
	inputDataRLP := []byte{}
	if ac.inputCount > 0 {
		if ac.input == nil {
			return i18n.NewError(ac.ctx, msgs.MsgEthClientMissingInput)
		}
		var inputUntyped any
		var cv *abi.ComponentValue
		switch input := ac.input.(type) {
		case map[string]any:
			inputUntyped = input
		case string:
			err = json.Unmarshal([]byte(input), &inputUntyped)
		case []byte:
			err = json.Unmarshal(input, &inputUntyped)
		case pldtypes.RawJSON:
			err = json.Unmarshal(input, &inputUntyped)
		case *abi.ComponentValue:
			cv = input
		default:
			var jsonInput []byte
			jsonInput, err = json.Marshal(ac.input)
			if err == nil {
				err = json.Unmarshal(jsonInput, &inputUntyped)
			}
		}
		if err == nil && cv == nil /* might have got a CV directly */ {
			cv, err = ac.inputs.ParseExternalCtx(ac.ctx, inputUntyped)
		}
		if err == nil {
			inputDataRLP, err = cv.EncodeABIDataCtx(ac.ctx)
		}
		if err != nil {
			return i18n.WrapError(ac.ctx, err, msgs.MsgEthClientInvalidInput, ac.signature)
		}
	}
	if ac.selector != nil {
		// function call
		ac.tx.Data = make([]byte, len(ac.selector)+len(inputDataRLP))
		copy(ac.tx.Data, ac.selector)
		copy(ac.tx.Data[len(ac.selector):], inputDataRLP)
	} else {
		// constructor
		ac.tx.Data = make([]byte, len(ac.bytecode)+len(inputDataRLP))
		copy(ac.tx.Data, ac.bytecode)
		copy(ac.tx.Data[len(ac.bytecode):], inputDataRLP)
	}
	return nil
}

func (ac *abiFunctionRequestBuilder) Call() (err error) {
	if ac.output == nil {
		return i18n.NewError(ac.ctx, msgs.MsgEthClientMissingOutput)
	}
	var jsonData []byte
	res, err := ac.CallResult()
	if err == nil {
		jsonData, err = res.DecodedResult.JSON()
	}
	if err == nil {
		err = json.Unmarshal(jsonData, ac.output)
	}
	if err != nil {
		return err
	}
	return nil
}

func (ac *abiFunctionRequestBuilder) callOps() []CallOption {
	return append([]CallOption{
		WithErrorsFrom(ac.abi),
		WithOutputs(ac.outputs),
		WithSerializer(ac.serializer),
	}, ac.extendedOpts...)
}

func (ac *abiFunctionRequestBuilder) CallResult() (res CallResult, err error) {
	err = ac.validateTo()
	if err == nil && ac.tx.Data == nil {
		err = ac.BuildCallData()
	}
	if err != nil {
		return res, err
	}
	return ac.ec.CallContract(ac.ctx, ac.fromStr, &ac.tx, ac.block, ac.callOps()...)
}

func (ac *abiFunctionRequestBuilder) EstimateGas() (res EstimateGasResult, err error) {
	err = ac.validateTo()
	if err == nil && ac.tx.Data == nil {
		err = ac.BuildCallData()
	}
	if err != nil {
		return res, err
	}
	return ac.ec.EstimateGas(ac.ctx, ac.fromStr, &ac.tx, ac.callOps()...)
}

func (ac *abiFunctionRequestBuilder) RawTransaction() (rawTX pldtypes.HexBytes, err error) {
	err = ac.validateTo()
	if err == nil && ac.tx.Data == nil {
		err = ac.BuildCallData()
	}
	if err != nil {
		return nil, err
	}
	if ac.fromStr == nil {
		return nil, i18n.NewError(ac.ctx, msgs.MsgEthClientMissingFrom)
	}
	return ac.ec.BuildRawTransaction(ac.ctx, ac.txVersion, *ac.fromStr, &ac.tx, ac.callOps()...)
}

func (ac *abiFunctionRequestBuilder) SignAndSend() (txHash *pldtypes.Bytes32, err error) {
	rawTX, err := ac.RawTransaction()
	if err != nil {
		return nil, err
	}
	return ac.ec.SendRawTransaction(ac.ctx, rawTX)
}
