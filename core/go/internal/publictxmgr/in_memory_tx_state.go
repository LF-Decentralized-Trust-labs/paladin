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

package publictxmgr

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
)

type managedTx struct {
	// persisted parts of the transaction
	ptx *DBPublicTxn

	// In-memory state that we update as we process the transaction in an active orchestrator
	InFlightStatus        InFlightStatus            // moves to pending/confirmed to cause the inflight to exit
	CurrentGasPrice       pldapi.PublicTxGasPricing // the gas price to use on the next transaction submission
	LastSubmittedGasPrice pldapi.PublicTxGasPricing // the gas price used on the last transaction submission
	Underpriced           bool                      // true if the last submitted gas price got an underpriced error
	TransactionHash       *pldtypes.Bytes32         // the most recently submitted transaction hash (not guaranteed to be the one mined)
	FirstSubmit           *pldtypes.Timestamp       // the time this runtime instance first did a submit JSON/RPC call (for success or failure)
	LastSubmit            *pldtypes.Timestamp       // the last time runtime instance first did a submit JSON/RPC call (for success or failure)
}

type inMemoryTxState struct {
	// reference back to the inFlightTransactionStageController
	*inFlightTransactionStageController

	managedTxMux sync.Mutex
	mtx          *managedTx
}

func gasPricingSet(gasPricing pldapi.PublicTxGasPricing) bool {
	return gasPricing.MaxFeePerGas != nil && gasPricing.MaxPriorityFeePerGas != nil
}

func NewInMemoryTxStateManager(ctx context.Context, ptx *DBPublicTxn, ift *inFlightTransactionStageController) InMemoryTxStateManager {
	imtxs := &inMemoryTxState{
		inFlightTransactionStageController: ift,
		mtx: &managedTx{
			ptx:            ptx,
			InFlightStatus: InFlightStatusPending,
		},
	}

	// Initialize the ephemeral state from the most recent persisted submission if one exists
	// This might occur if a Paladin node is restarted after a transaction has been submitted
	// or if an orchestrator is swapped out under heavy load
	// Note that the submissions list is not kept up to date in the in-memory state
	if len(ptx.Submissions) > 0 {
		lastSub := ptx.Submissions[0]
		imtxs.mtx.TransactionHash = &lastSub.TransactionHash
		imtxs.mtx.LastSubmit = &lastSub.Created
		firstSub := ptx.Submissions[len(ptx.Submissions)-1]
		imtxs.mtx.FirstSubmit = &firstSub.Created
		imtxs.mtx.LastSubmittedGasPrice = recoverGasPriceOptions(lastSub.GasPricing)
	}
	return imtxs
}

func (imtxs *inMemoryTxState) UpdateTransaction(ctx context.Context, newPtx *DBPublicTxn) {
	imtxs.managedTxMux.Lock()
	defer imtxs.managedTxMux.Unlock()

	ptx := imtxs.mtx.ptx
	ptx.To = newPtx.To
	ptx.Data = newPtx.Data
	ptx.Gas = newPtx.Gas
	ptx.FixedGasPricing = newPtx.FixedGasPricing
	ptx.Value = newPtx.Value
}

func (imtxs *inMemoryTxState) ApplyInMemoryUpdates(ctx context.Context, txUpdates *BaseTXUpdates) {
	imtxs.managedTxMux.Lock()
	defer imtxs.managedTxMux.Unlock()

	mtx := imtxs.mtx

	newValues := txUpdates.NewValues
	resetValues := txUpdates.ResetValues

	if newValues.FirstSubmit != nil {
		mtx.FirstSubmit = newValues.FirstSubmit
	}

	if newValues.GasPricing != nil {
		mtx.CurrentGasPrice = *newValues.GasPricing
	}

	if newValues.Underpriced != nil {
		mtx.Underpriced = *newValues.Underpriced
	}

	if newValues.NewSubmission != nil {
		mtx.LastSubmittedGasPrice = recoverGasPriceOptions(newValues.NewSubmission.GasPricing)
	}

	if newValues.LastSubmit != nil {
		mtx.LastSubmit = newValues.LastSubmit
	}

	if newValues.InFlightStatus != nil {
		mtx.InFlightStatus = *newValues.InFlightStatus
	}

	if newValues.TransactionHash != nil {
		mtx.TransactionHash = newValues.TransactionHash
	}

	if resetValues.GasPricing {
		mtx.CurrentGasPrice = pldapi.PublicTxGasPricing{}
	}

	if resetValues.TransactionHash {
		mtx.TransactionHash = nil
	}

	if resetValues.Underpriced {
		mtx.Underpriced = false
	}
}

func (imtxs *inMemoryTxState) GetPubTxnID() uint64 {
	return imtxs.mtx.ptx.PublicTxnID
}

func (imtxs *inMemoryTxState) GetSignerNonce() string {
	nonceStr := "unassigned"
	if imtxs.mtx.ptx.Nonce != nil {
		nonceStr = strconv.FormatUint(*imtxs.mtx.ptx.Nonce, 10)
	}
	return fmt.Sprintf("%s:%s", imtxs.mtx.ptx.From, nonceStr)
}

func (imtxs *inMemoryTxState) GetCreatedTime() *pldtypes.Timestamp {
	return &imtxs.mtx.ptx.Created
}

func (imtxs *inMemoryTxState) GetTransactionHash() *pldtypes.Bytes32 {
	return imtxs.mtx.TransactionHash
}

func (imtxs *inMemoryTxState) GetNonce() uint64 {
	return *imtxs.mtx.ptx.Nonce
}

func (imtxs *inMemoryTxState) GetFrom() pldtypes.EthAddress {
	return imtxs.mtx.ptx.From
}

func (imtxs *inMemoryTxState) GetTo() *pldtypes.EthAddress {

	return imtxs.mtx.ptx.To
}

func (imtxs *inMemoryTxState) GetValue() *pldtypes.HexUint256 {
	return imtxs.mtx.ptx.Value
}

func (imtxs *inMemoryTxState) BuildEthTX() *ethsigner.Transaction {
	// Builds the ethereum TX using the latest in-memory information that must have been resolved in previous stages
	ptx := imtxs.mtx.ptx
	return buildEthTX(
		ptx.From,
		ptx.Nonce,
		ptx.To,
		ptx.Data,
		&pldapi.PublicTxOptions{
			Gas:                (*pldtypes.HexUint64)(&ptx.Gas), // fixed in persisted TX
			Value:              ptx.Value,
			PublicTxGasPricing: imtxs.mtx.CurrentGasPrice, // variable and calculated in memory
		},
	)
}

func (imtxs *inMemoryTxState) GetFirstSubmit() *pldtypes.Timestamp {
	return imtxs.mtx.FirstSubmit
}

func (imtxs *inMemoryTxState) GetGasPriceObject() *pldapi.PublicTxGasPricing {
	if gasPricingSet(imtxs.mtx.CurrentGasPrice) {
		return &imtxs.mtx.CurrentGasPrice
	}
	// no gas price set yet, return nil, down stream logic relies on `nil` to know a transaction isn't currently assigned a gas price.
	return nil
}

func (imtxs *inMemoryTxState) GetTransactionFixedGasPrice() *pldapi.PublicTxGasPricing {
	fixedPrice := recoverGasPriceOptions(imtxs.mtx.ptx.FixedGasPricing)
	if gasPricingSet(fixedPrice) {
		return &fixedPrice
	}
	return nil
}

func (imtxs *inMemoryTxState) GetLastSubmittedGasPrice() *pldapi.PublicTxGasPricing {
	return &imtxs.mtx.LastSubmittedGasPrice
}

func (imtxs *inMemoryTxState) GetUnderpriced() bool {
	return imtxs.mtx.Underpriced
}

func (imtxs *inMemoryTxState) GetLastSubmitTime() *pldtypes.Timestamp {
	return imtxs.mtx.LastSubmit
}

func (imtxs *inMemoryTxState) GetGasLimit() uint64 {
	return imtxs.mtx.ptx.Gas
}

func (imtxs *inMemoryTxState) GetInFlightStatus() InFlightStatus {
	return imtxs.mtx.InFlightStatus
}

func (imtxs *inMemoryTxState) IsReadyToExit() bool {
	return imtxs.mtx.InFlightStatus != InFlightStatusPending
}

func NewRunningStageContext(ctx context.Context, stage InFlightTxStage, substatus BaseTxSubStatus, imtxs InMemoryTxStateManager) *RunningStageContext {
	return &RunningStageContext{
		Stage:          stage,
		SubStatus:      substatus,
		StageOutput:    &StageOutput{},
		Context:        ctx,
		StageStartTime: time.Now(),
		InMemoryTx:     imtxs,
	}
}
