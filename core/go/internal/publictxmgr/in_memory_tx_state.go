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

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
)

type managedTx struct {
	// persisted parts of the transaction, and the list of flushed DB submissions
	ptx *DBPublicTxn

	// We can have exactly one submission waiting to be flushed to the DB
	unflushedSubmission *DBPubTxnSubmission

	// In-memory state that we update as we process the transaction in an active orchestrator
	InFlightStatus  InFlightStatus            // moves to pending/confirmed to cause the inflight to exit
	GasPricing      pldapi.PublicTxGasPricing // the most recently used gas pricing information
	TransactionHash *pldtypes.Bytes32         // the most recently submitted transaction hash (not guaranteed to be the one mined)
	FirstSubmit     *pldtypes.Timestamp       // the time this runtime instance first did a submit JSON/RPC call (for success or failure)
	LastSubmit      *pldtypes.Timestamp       // the last time runtime instance first did a submit JSON/RPC call (for success or failure)
}

type inMemoryTxState struct {
	managedTxMux sync.Mutex
	mtx          *managedTx
}

func gasPricingSet(gasPricing pldapi.PublicTxGasPricing) bool {
	return gasPricing.GasPrice != nil || gasPricing.MaxFeePerGas != nil || gasPricing.MaxPriorityFeePerGas != nil
}

func NewInMemoryTxStateManager(ctx context.Context, ptx *DBPublicTxn) InMemoryTxStateManager {
	imtxs := &inMemoryTxState{
		mtx: &managedTx{
			ptx:            ptx,
			InFlightStatus: InFlightStatusPending,
			GasPricing:     recoverGasPriceOptions(ptx.FixedGasPricing),
		},
	}

	// Initialize the ephemeral state from the most recent persisted submission if one exists
	if len(ptx.Submissions) > 0 {
		lastSub := ptx.Submissions[0]
		imtxs.mtx.TransactionHash = &lastSub.TransactionHash
		imtxs.mtx.LastSubmit = &lastSub.Created
		firstSub := ptx.Submissions[len(ptx.Submissions)-1]
		imtxs.mtx.FirstSubmit = &firstSub.Created
		if imtxs.GetGasPriceObject() == nil {
			lastGasPricing := recoverGasPriceOptions(lastSub.GasPricing)
			imtxs.mtx.GasPricing = lastGasPricing
		}
	}
	return imtxs
}

func (imtxs *inMemoryTxState) UpdateTransaction(newPtx *DBPublicTxn) {
	imtxs.managedTxMux.Lock()
	defer imtxs.managedTxMux.Unlock()
	// If this update didn't involve a change to how fixed gas pricing is set (i.e. it wasn't set before
	// and it isn't set now) then we don't want to change the gas pricing back to empty because
	// it could result in us retrieving a gas price that is lower than one we've already submitted
	newFixedPricing := recoverGasPriceOptions(newPtx.FixedGasPricing)
	oldFixedPricing := recoverGasPriceOptions(imtxs.mtx.ptx.FixedGasPricing)
	if gasPricingSet(newFixedPricing) || gasPricingSet(oldFixedPricing) {
		imtxs.mtx.GasPricing = newFixedPricing
	}

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

	if txUpdates.FirstSubmit != nil {
		mtx.FirstSubmit = txUpdates.FirstSubmit
	}

	if txUpdates.GasPricing != nil {
		mtx.GasPricing = *txUpdates.GasPricing
	}

	if txUpdates.NewSubmission != nil {
		imtxs.mtx.unflushedSubmission = txUpdates.NewSubmission
	}
	if txUpdates.FlushedSubmission != nil {
		// We're being notified some of the unflushed submissions have been flushed to persistence
		// We clear the flushing list and merge in these new ones
		dup := false
		for _, existing := range mtx.ptx.Submissions {
			if existing.TransactionHash == txUpdates.FlushedSubmission.TransactionHash {
				dup = true
				break
			}
		}
		if !dup {
			// newest first in this list as when we read from the DB (although it doesn't matter for our processing,
			// because we keep separate in memory copies of all the things we change while we're running our orchestrator)
			mtx.ptx.Submissions = append([]*DBPubTxnSubmission{txUpdates.FlushedSubmission}, mtx.ptx.Submissions...)
		}
	}

	if txUpdates.LastSubmit != nil {
		mtx.LastSubmit = txUpdates.LastSubmit
	}

	if txUpdates.InFlightStatus != nil {
		mtx.InFlightStatus = *txUpdates.InFlightStatus
	}

	if txUpdates.TransactionHash != nil {
		mtx.TransactionHash = txUpdates.TransactionHash
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

func (imtxs *inMemoryTxState) ResetTransactionHash() {
	imtxs.mtx.TransactionHash = nil
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
			PublicTxGasPricing: imtxs.mtx.GasPricing, // variable and calculated in memory
		},
	)
}

func (imtxs *inMemoryTxState) GetFirstSubmit() *pldtypes.Timestamp {
	return imtxs.mtx.FirstSubmit
}

func (imtxs *inMemoryTxState) GetGasPriceObject() *pldapi.PublicTxGasPricing {
	if gasPricingSet(imtxs.mtx.GasPricing) {
		return &imtxs.mtx.GasPricing
	}
	// no gas price set yet, return nil, down stream logic relies on `nil` to know a transaction has never been assigned any gas price.
	return nil
}

func (imtxs *inMemoryTxState) GetLastSubmitTime() *pldtypes.Timestamp {
	return imtxs.mtx.LastSubmit
}

func (imtxs *inMemoryTxState) GetUnflushedSubmission() *DBPubTxnSubmission {
	return imtxs.mtx.unflushedSubmission
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
