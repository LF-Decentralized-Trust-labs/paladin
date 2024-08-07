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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type ABIFunctionClient[IN, OUT any] interface {
	R(ctx context.Context) ABIFunctionRequestBuilder[IN, OUT]
}

type EthTXVersion int

const (
	LEGACY_ORIGINAL EthTXVersion = 1
	LEGACY_EIP155   EthTXVersion = 2
	EIP1559         EthTXVersion = 3
)

type ABIFunctionRequestBuilder[IN, OUT any] interface {
	TXVersion(EthTXVersion) ABIFunctionRequestBuilder[IN, OUT]
	Signer(string) ABIFunctionRequestBuilder[IN, OUT]
	To(*ethtypes.Address0xHex) ABIFunctionRequestBuilder[IN, OUT]
	GasLimit(*big.Int) ABIFunctionRequestBuilder[IN, OUT]
	Input(*IN) ABIFunctionRequestBuilder[IN, OUT]

	Call() (data *OUT, err error)
	CallJSON() (jsonData []byte, err error)
	RawTransaction() (rawTX ethtypes.HexBytes0xPrefix, err error)
	SignAndSend() (txHash ethtypes.HexBytes0xPrefix, err error)
}

// For typing a nil return from a ABIFunctionClient
type NONE struct{}

type abiFunctionClient[IN, OUT any] struct {
	ec        *ethClient
	signature string
	selector  []byte
	inputs    abi.TypeComponent
	outputs   abi.TypeComponent
}

type abiFunctionRequestBuilder[IN, OUT any] struct {
	*abiFunctionClient[IN, OUT]
	ctx       context.Context
	txVersion EthTXVersion
	tx        ethsigner.Transaction
	fromStr   *string
	input     *IN
}

func WrapFunction[IN, OUT any](ctx context.Context, ec EthClient, functionABI *abi.Entry) (_ ABIFunctionClient[IN, OUT], err error) {
	ac := &abiFunctionClient[IN, OUT]{ec: ec.(*ethClient)}
	ac.selector, err = functionABI.GenerateFunctionSelectorCtx(ctx)
	if err == nil {
		ac.signature, err = functionABI.Signature()
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

func MustWrapFunction[IN, OUT any](ctx context.Context, ec EthClient, functionABI *abi.Entry) ABIFunctionClient[IN, OUT] {
	ac, err := WrapFunction[IN, OUT](ctx, ec, functionABI)
	if err != nil {
		panic(err)
	}
	return ac
}

func (ac *abiFunctionClient[IN, OUT]) R(ctx context.Context) ABIFunctionRequestBuilder[IN, OUT] {
	return &abiFunctionRequestBuilder[IN, OUT]{
		txVersion:         EIP1559,
		abiFunctionClient: ac,
		ctx:               ctx,
	}
}

func (ac *abiFunctionRequestBuilder[IN, OUT]) TXVersion(v EthTXVersion) ABIFunctionRequestBuilder[IN, OUT] {
	ac.txVersion = v
	return ac
}

func (ac *abiFunctionRequestBuilder[IN, OUT]) Signer(fromStr string) ABIFunctionRequestBuilder[IN, OUT] {
	ac.fromStr = &fromStr
	return ac
}

func (ac *abiFunctionRequestBuilder[IN, OUT]) To(to *ethtypes.Address0xHex) ABIFunctionRequestBuilder[IN, OUT] {
	ac.tx.To = to
	return ac
}

func (ac *abiFunctionRequestBuilder[IN, OUT]) GasLimit(gas *big.Int) ABIFunctionRequestBuilder[IN, OUT] {
	ac.tx.GasLimit = (*ethtypes.HexInteger)(gas)
	return ac
}

func (ac *abiFunctionRequestBuilder[IN, OUT]) Input(input *IN) ABIFunctionRequestBuilder[IN, OUT] {
	ac.input = input
	return ac
}

func (ac *abiFunctionRequestBuilder[IN, OUT]) BuildCallData() error {
	if ac.input == nil {
		return i18n.NewError(ac.ctx, msgs.MsgEthClientMissingInput)
	}
	if ac.tx.To == nil {
		return i18n.NewError(ac.ctx, msgs.MsgEthClientMissingTo)
	}
	// Encode the call data
	var inputMap map[string]interface{}
	var jsonInput []byte
	jsonInput, err := json.Marshal(ac.input)
	if err == nil {
		err = json.Unmarshal(jsonInput, &inputMap)
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

func (ac *abiFunctionRequestBuilder[IN, OUT]) Call() (data *OUT, err error) {
	jsonData, err := ac.CallJSON()
	if err == nil {
		err = json.Unmarshal(jsonData, &data)
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (ac *abiFunctionRequestBuilder[IN, OUT]) CallJSON() (jsonData []byte, err error) {
	if ac.tx.Data == nil {
		if err := ac.BuildCallData(); err != nil {
			return nil, err
		}
	}
	resData, err := ac.ec.CallContract(ac.ctx, ac.fromStr, &ac.tx)
	if err != nil {
		return nil, err
	}
	cv, err := ac.outputs.DecodeABIDataCtx(ac.ctx, resData, 0)
	if err == nil {
		jsonData, err = types.StandardABISerializer().SerializeJSONCtx(ac.ctx, cv)
	}
	return jsonData, err
}

func (ac *abiFunctionRequestBuilder[IN, OUT]) RawTransaction() (rawTX ethtypes.HexBytes0xPrefix, err error) {
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

func (ac *abiFunctionRequestBuilder[IN, OUT]) SignAndSend() (txHash ethtypes.HexBytes0xPrefix, err error) {
	rawTX, err := ac.RawTransaction()
	if err != nil {
		return nil, err
	}
	return ac.ec.SendRawTransaction(ac.ctx, rawTX)
}
