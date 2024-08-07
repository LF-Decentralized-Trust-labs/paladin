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
	"math/big"
	"strconv"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type ABIFunctionClient interface {
	R(ctx context.Context) ABIFunctionRequestBuilder
}

type EthTXVersion int

const (
	LEGACY_ORIGINAL EthTXVersion = 1
	LEGACY_EIP155   EthTXVersion = 2
	EIP1559         EthTXVersion = 3
)

type ABIClient interface {
	ABI() abi.ABI
	Function(ctx context.Context, nameOrFullSig string) (_ ABIFunctionClient, err error)
	MustFunction(nameOrFullSig string) ABIFunctionClient
}

type ABIFunctionRequestBuilder interface {
	TXVersion(EthTXVersion) ABIFunctionRequestBuilder
	Signer(string) ABIFunctionRequestBuilder
	To(*ethtypes.Address0xHex) ABIFunctionRequestBuilder
	GasLimit(*big.Int) ABIFunctionRequestBuilder
	BlockRef(blockRef BlockRef) ABIFunctionRequestBuilder
	Block(uint64) ABIFunctionRequestBuilder
	Input(any) ABIFunctionRequestBuilder
	Output(any) ABIFunctionRequestBuilder

	Call() (err error)
	CallJSON() (jsonData []byte, err error)
	RawTransaction() (rawTX ethtypes.HexBytes0xPrefix, err error)
	SignAndSend() (txHash ethtypes.HexBytes0xPrefix, err error)
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
	ec        *ethClient
	signature string
	selector  []byte
	inputs    abi.TypeComponent
	outputs   abi.TypeComponent
}

type abiFunctionRequestBuilder struct {
	*abiFunctionClient
	ctx       context.Context
	txVersion EthTXVersion
	tx        ethsigner.Transaction
	block     string
	fromStr   *string
	input     any
	output    any
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
	functionABI := abic.functions[nameOrFullSig]
	if functionABI == nil {
		return nil, i18n.NewError(ctx, msgs.MsgEthClientFunctionNotFound, nameOrFullSig)
	}
	ac := &abiFunctionClient{ec: abic.ec}
	ac.selector, err = functionABI.GenerateFunctionSelectorCtx(ctx)
	if err == nil {
		ac.signature, err = functionABI.SignatureCtx(ctx)
	}
	if err == nil {
		ac.inputs, err = functionABI.Inputs.TypeComponentTreeCtx(ctx)
	}
	if err == nil {
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

func (abic *abiClient) ABI() abi.ABI {
	return abic.abi
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

func (ac *abiFunctionRequestBuilder) GasLimit(gas *big.Int) ABIFunctionRequestBuilder {
	ac.tx.GasLimit = (*ethtypes.HexInteger)(gas)
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

func (ac *abiFunctionRequestBuilder) BuildCallData() (err error) {
	if ac.input == nil {
		return i18n.NewError(ac.ctx, msgs.MsgEthClientMissingInput)
	}
	if ac.tx.To == nil {
		return i18n.NewError(ac.ctx, msgs.MsgEthClientMissingTo)
	}
	// Encode the call data
	var inputMap map[string]any
	switch input := ac.input.(type) {
	case map[string]any:
		inputMap = input
	case string:
		err = json.Unmarshal([]byte(input), &inputMap)
	case []byte:
		err = json.Unmarshal(input, &inputMap)
	default:
		var jsonInput []byte
		jsonInput, err = json.Marshal(ac.input)
		if err == nil {
			err = json.Unmarshal(jsonInput, &inputMap)
		}
	}
	var cv *abi.ComponentValue
	if err == nil {
		cv, err = ac.inputs.ParseExternalCtx(ac.ctx, inputMap)
	}
	var inputDataRLP []byte
	if err == nil {
		inputDataRLP, err = cv.EncodeABIDataCtx(ac.ctx)
	}
	if err != nil {
		return i18n.WrapError(ac.ctx, err, msgs.MsgEthClientInvalidInput, ac.signature)
	}
	ac.tx.Data = make([]byte, len(ac.selector)+len(inputDataRLP))
	copy(ac.tx.Data, ac.selector)
	copy(ac.tx.Data[len(ac.selector):], inputDataRLP)
	return nil
}

func (ac *abiFunctionRequestBuilder) Call() (err error) {
	if ac.output == nil {
		return i18n.NewError(ac.ctx, msgs.MsgEthClientMissingOutput)
	}
	jsonData, err := ac.CallJSON()
	if err == nil {
		err = json.Unmarshal(jsonData, ac.output)
	}
	if err != nil {
		return err
	}
	return nil
}

func (ac *abiFunctionRequestBuilder) CallJSON() (jsonData []byte, err error) {
	if ac.tx.Data == nil {
		if err := ac.BuildCallData(); err != nil {
			return nil, err
		}
	}
	resData, err := ac.ec.CallContract(ac.ctx, ac.fromStr, &ac.tx, ac.block)
	if err != nil {
		return nil, err
	}
	cv, err := ac.outputs.DecodeABIDataCtx(ac.ctx, resData, 0)
	if err == nil {
		jsonData, err = types.StandardABISerializer().SerializeJSONCtx(ac.ctx, cv)
	}
	return jsonData, err
}

func (ac *abiFunctionRequestBuilder) RawTransaction() (rawTX ethtypes.HexBytes0xPrefix, err error) {
	if ac.tx.Data == nil {
		if err := ac.BuildCallData(); err != nil {
			return nil, err
		}
	}
	if ac.fromStr == nil {
		return nil, i18n.NewError(ac.ctx, msgs.MsgEthClientMissingFrom)
	}
	return ac.ec.BuildRawTransaction(ac.ctx, ac.txVersion, *ac.fromStr, &ac.tx)
}

func (ac *abiFunctionRequestBuilder) SignAndSend() (txHash ethtypes.HexBytes0xPrefix, err error) {
	rawTX, err := ac.RawTransaction()
	if err != nil {
		return nil, err
	}
	return ac.ec.SendRawTransaction(ac.ctx, rawTX)
}
