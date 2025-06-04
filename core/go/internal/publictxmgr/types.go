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
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
)

// TXUpdates specifies a set of updates that are possible on the base structure.
//
// Any non-nil fields will be set.
// Sub-objects are set as a whole, apart from TransactionHeaders where each field
// is considered and stored individually.
// JSONAny fields can be set explicitly to null using fftypes.NullString
//
// This is the update interface for the policy engine to update base status on the
// transaction object.
//
// There are separate setter functions for fields that depending on the persistence
// mechanism might be in separate tables - including History, Receipt, and Confirmations
type BaseTXUpdates struct {
	InFlightStatus *InFlightStatus
	SubStatus      *BaseTxSubStatus
	GasPricing     *pldapi.PublicTxGasPricing
	// GasLimit          *pldtypes.HexUint64 // note this is required for some methods (eth_estimateGas)
	TransactionHash   *pldtypes.Bytes32
	FirstSubmit       *pldtypes.Timestamp
	LastSubmit        *pldtypes.Timestamp
	ErrorMessage      *string
	NewSubmission     *DBPubTxnSubmission
	FlushedSubmission *DBPubTxnSubmission
}

// PublicTransactionEventType is a enum type that contains all types of transaction process events
// that a transaction handler emits.
type PublicTransactionEventType int

const (
	PublicTXProcessSucceeded PublicTransactionEventType = iota
	PublicTXProcessFailed
)

type RequestOptions struct {
	ID       *uuid.UUID
	SignerID string
	GasLimit *ethtypes.HexInteger
}

// BaseTxSubStatus is an intermediate status a transaction may go through
type BaseTxSubStatus string

const (
	// BaseTxSubStatusReceived indicates the transaction has been received by the connector
	BaseTxSubStatusReceived BaseTxSubStatus = "Received"
	// BaseTxSubStatusStale indicates the transaction is now in stale
	BaseTxSubStatusStale BaseTxSubStatus = "Stale"
	// BaseTxSubStatusTracking indicates we are tracking progress of the transaction
	BaseTxSubStatusTracking BaseTxSubStatus = "Tracking"
	// BaseTxSubStatusConfirmed indicates we have confirmed that the transaction has been fully processed
	BaseTxSubStatusConfirmed BaseTxSubStatus = "Confirmed"
)

type BaseTxAction string

const (
	// BaseTxActionSign indicates the operation has been signed
	BaseTxActionSign BaseTxAction = "Sign"
)

const (
	// BaseTxActionStateTransition is a special value used for state transition entries, which are created using SetSubStatus
	BaseTxActionStateTransition BaseTxAction = "StateTransition"
	// BaseTxActionAssignNonce indicates that a nonce has been assigned to the transaction
	BaseTxActionAssignNonce BaseTxAction = "AssignNonce"
	// BaseTxActionRetrieveGasPrice indicates the operation is getting a gas price
	BaseTxActionRetrieveGasPrice BaseTxAction = "RetrieveGasPrice"
	// BaseTxActionSubmitTransaction indicates that the transaction has been submitted
	BaseTxActionSubmitTransaction BaseTxAction = "SubmitTransaction"
	// BaseTxActionConfirmTransaction indicates that the transaction has been confirmed
	BaseTxActionConfirmTransaction BaseTxAction = "Confirm"
)

type TransactionHeaders struct {
	From  string            `json:"from,omitempty"`
	To    string            `json:"to,omitempty"`
	Nonce *fftypes.FFBigInt `json:"nonce,omitempty"`
	Gas   *fftypes.FFBigInt `json:"gas,omitempty"`
	Value *fftypes.FFBigInt `json:"value,omitempty"`
}

type BalanceManager interface {
	GetAddressBalance(ctx context.Context, address pldtypes.EthAddress) (*AddressAccount, error)
	NotifyAddressBalanceChanged(ctx context.Context, address pldtypes.EthAddress)
}

// AddressAccount provides the following feature:
// - record a snapshot of the current balance of a signing address
// - record the total spent of a series of transaction emitted by this signing address
// - provide an interface to top up the signing address when spent is higher than the balance
type AddressAccount struct {
	Address               pldtypes.EthAddress
	Balance               *big.Int
	SpentTransactionCount int
	MinCost               *big.Int
	MaxCost               *big.Int
	Spent                 *big.Int
}

func (ab *AddressAccount) Spend(ctx context.Context, cost *big.Int) (availableToSpend *big.Int) {

	if cost.Sign() == 1 {
		ab.Spent = ab.Spent.Add(ab.Spent, cost)
		if ab.MinCost.Sign() == 0 || ab.MinCost.Cmp(cost) > 0 {
			ab.MinCost = cost
		}

		if ab.MaxCost.Sign() == 0 || ab.MaxCost.Cmp(cost) < 0 {
			ab.MaxCost = cost
		}
		// only increase the transaction count if there is a positive cost number
		ab.SpentTransactionCount = ab.SpentTransactionCount + 1
	} else if cost.Sign() == -1 {
		// cost cannot be a negative value...
		log.L(ctx).Errorf("Received a negative number for transaction cost: %s, ignore the cost and keep address balance unchanged.", cost.String())
	}
	availableToSpend = ab.GetAvailableToSpend(ctx)
	return availableToSpend
}

func (ab *AddressAccount) GetAvailableToSpend(ctx context.Context) *big.Int {
	balanceCopy := new(big.Int).Set(ab.Balance)
	return balanceCopy.Sub(balanceCopy, ab.Spent)
}

type Confirmation struct {
	BlockNumber fftypes.FFuint64 `json:"blockNumber"`
	BlockHash   string           `json:"blockHash"`
	ParentHash  string           `json:"parentHash"`
}

type ConfirmationsNotification struct {
	// Confirmed marks we've reached the confirmation threshold
	Confirmed bool
	// NewFork is true when NewConfirmations is a complete list of confirmations.
	// Otherwise, Confirmations is an additive delta on top of a previous list of confirmations.
	NewFork bool
	// Confirmations is the list of confirmations being notified - assured to be non-nil, but might be empty.
	Confirmations []*Confirmation
}

// in flight tx stages are calculated based on a snapshot of a persisted managed transaction
//
//	an in flight stage can contain 1 or more sub-status updates
type InFlightTxStage string

const (
	// managed transaction criteria for entering each stage:
	//   entry criteria:
	//     transaction in a state that requires status update
	//   emitted async actions (in parallel):
	//     status update
	//   completion criteria
	//     status update completed
	InFlightTxStageStatusUpdate InFlightTxStage = "statusUpdate"

	//   entry criteria (OR):
	//     no gas price substatus since the most recent expired "submit" / "receipt" substatus or the beginning of substatus array
	//     no gas limit since the most recent expired "submit" / "receipt" substatus or the beginning of substatus array
	//   emitted async actions (in parallel):
	//     - retrieve gas price
	//     - get gas limit (TBD, current the gas estimation only happens before transaction process, need to fix this)
	//   completion criteria -> sign
	//     valid gas price and gas limit substatus since the most recent expired "submit" / "receipt" substatus or the beginning of substatus array
	InFlightTxStageRetrieveGasPrice InFlightTxStage = "retrieveGasPrice"
	//   entry criteria (AND):
	//     valid gas price and gas limit substatus since the most recent expired "submit" / "receipt" substatus or the beginning of substatus array
	//     no completed "sign" substatus with signed message value since above criteria
	//   emitted async actions:
	//     - signing request
	//   completion criteria -> submit
	//     the last sub-status is a completed (success) "sign" substatus
	InFlightTxStageSigning InFlightTxStage = "sign"
	//   entry criteria (OR):
	//     the last sub-status is a completed "sign" substatus
	//     the last sub-status is an incomplete "submit" substatus
	//   emitted async actions:
	//     - transaction submission
	//          wait condition when balance check turned on:
	//            - in-sufficient balance
	//            - cost of transaction with previous nonce unknown
	//   completion criteria -> receipt / prepare
	//     the last sub-status is a completed (success/expired) "submit" substatus
	InFlightTxStageSubmitting InFlightTxStage = "submit"
	//   entry criteria(OR):
	//     the last sub-status is a completed "confirmed" substatus
	//   end of lifecycle, rely on transaction engine to remove the item from the queue
	InFlightTxStageComplete InFlightTxStage = "complete"

	//   entry criteria:
	//     not in other state
	//   completion criteria
	//      evaluated into other state
	InFlightTxStageQueued InFlightTxStage = "queued"
)

var AllInFlightStages = []string{
	string(InFlightTxStageRetrieveGasPrice),
	string(InFlightTxStageSigning),
	string(InFlightTxStageSubmitting),
	string(InFlightTxStageComplete),
	string(InFlightTxStageQueued),
}

type SubmissionOutcome string

const (
	// continue cases
	SubmissionOutcomeSubmittedNew SubmissionOutcome = "submittedNew"
	SubmissionOutcomeNonceTooLow  SubmissionOutcome = "nonceTooLow"
	SubmissionOutcomeAlreadyKnown SubmissionOutcome = "alreadyKnown"

	// error cases
	SubmissionOutcomeFailedRequiresRetry SubmissionOutcome = "errRequiresRetry"
)

type InMemoryTxStateReadOnly interface {
	GetCreatedTime() *pldtypes.Timestamp
	// get the transaction receipt from the in-memory state (note: the returned value should not be modified)
	GetTransactionHash() *pldtypes.Bytes32
	GetPubTxnID() uint64
	GetNonce() uint64
	GetFrom() pldtypes.EthAddress
	GetTo() *pldtypes.EthAddress
	GetValue() *pldtypes.HexUint256
	BuildEthTX() *ethsigner.Transaction
	GetGasPriceObject() *pldapi.PublicTxGasPricing
	GetFirstSubmit() *pldtypes.Timestamp
	GetLastSubmitTime() *pldtypes.Timestamp
	GetUnflushedSubmission() *DBPubTxnSubmission
	GetInFlightStatus() InFlightStatus
	GetSignerNonce() string
	GetGasLimit() uint64
	IsReadyToExit() bool
}

type InMemoryTxStateManager interface {
	InMemoryTxStateReadOnly
	InMemoryTxStateSetters
}

type InMemoryTxStateSetters interface {
	ApplyInMemoryUpdates(ctx context.Context, txUpdates *BaseTXUpdates)
	UpdateTransaction(newPtx *DBPublicTxn)
	ResetTransactionHash()
}

type StageOutput struct {
	Stage InFlightTxStage

	PersistenceOutput *PersistenceOutput

	SubmitOutput *SubmitOutputs

	SignOutput *SignOutputs

	GasPriceOutput *GasPriceOutput

	ConfirmationOutput *ConfirmationOutputs
}

type SubmitOutputs struct {
	TxHash            *pldtypes.Bytes32
	SubmissionTime    *pldtypes.Timestamp
	SubmissionOutcome SubmissionOutcome
	ErrorReason       string
	Err               error
}
type SignOutputs struct {
	SignedMessage []byte
	TxHash        *pldtypes.Bytes32
	Err           error
}

type GasPriceOutput struct {
	GasPriceObject *pldapi.PublicTxGasPricing
	Err            error
}

type ConfirmationOutputs struct {
	/* no outputs as we never exit this stage through stage processing */
}

type PersistenceOutput struct {
	PersistenceError error
	Time             time.Time
}

type InFlightStageActionTriggers interface {
	TriggerRetrieveGasPrice(ctx context.Context) error
	TriggerSignTx(ctx context.Context) error
	TriggerSubmitTx(ctx context.Context, signedMessage []byte, calculatedTxHash *pldtypes.Bytes32) error
	TriggerStatusUpdate(ctx context.Context) error
}

// RunningStageContext is the context for an individual run of the transaction process
type RunningStageContext struct {
	InMemoryTx InMemoryTxStateReadOnly
	context.Context
	Stage          InFlightTxStage
	SubStatus      BaseTxSubStatus
	StageStartTime time.Time
	StageErrored   bool

	// for some stages, it requires inputs from multiple async request
	// this object is used to accumulate all the request result
	// the stage processing logic can decide whether there is a need to use this
	StageOutput *StageOutput

	StageOutputsToBePersisted *RunningStageContextPersistenceOutput
}

func (ctx *RunningStageContext) SetSubStatus(subStatus BaseTxSubStatus) {
	ctx.SubStatus = subStatus
}

// This function records that there's updates (in-memory and/or persistence) that are coming from
// a stage and need the primary routine to flush it into the in-memory model.
func (ctx *RunningStageContext) SetNewPersistenceUpdateOutput() {
	if ctx.StageOutputsToBePersisted == nil {
		ctx.StageOutputsToBePersisted = &RunningStageContextPersistenceOutput{
			InMemoryTx: ctx.InMemoryTx,
			SubStatus:  ctx.SubStatus,
			Ctx:        ctx,
		}
	}
}

type StatusUpdater interface {
	UpdateSubStatus(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info *fftypes.JSONAny, err *fftypes.JSONAny, actionOccurred *pldtypes.Timestamp) error
}

type RunningStageContextPersistenceOutput struct {
	InMemoryTx    InMemoryTxStateReadOnly
	SubStatus     BaseTxSubStatus
	Ctx           context.Context
	TxUpdates     *BaseTXUpdates
	StatusUpdates []func(p StatusUpdater) error
}

func (sOut *RunningStageContextPersistenceOutput) UpdateSubStatus(action BaseTxAction, info *fftypes.JSONAny, err *fftypes.JSONAny) {
	actionOccurred := pldtypes.TimestampNow()
	sOut.StatusUpdates = append(sOut.StatusUpdates, func(p StatusUpdater) error {
		return p.UpdateSubStatus(sOut.Ctx, sOut.InMemoryTx, sOut.SubStatus, action, info, err, &actionOccurred)
	})
}

type OrchestratorContext struct {
	// input from transaction engine
	AvailableToSpend         *big.Int
	PreviousNonceCostUnknown bool
}

// output of some stages doesn't get written into the database
// so it needs to be carried over to next stages
type TransientPreviousStageOutputs struct {
	SignedMessage   []byte // NB: if the value is nil when triggering submitTx , node signer will be used to sign the transaction instead, don't use this to judge whether a transaction can be submitted or not.
	TransactionHash *pldtypes.Bytes32
}

type InFlightTransactionStateManager interface {
	// tx state management
	InMemoryTxStateReadOnly
	InMemoryTxStateSetters
	CanSubmit(ctx context.Context, cost *big.Int) bool
	CanBeRemoved(ctx context.Context) bool
	GetInFlightStatus() InFlightStatus
	SetOrchestratorContext(ctx context.Context, tec *OrchestratorContext)
	GetStage(ctx context.Context) InFlightTxStage

	// genereation management
	GetGenerations(ctx context.Context) []InFlightTransactionStateGeneration
	GetGeneration(ctx context.Context, id int) InFlightTransactionStateGeneration
	GetCurrentGeneration(ctx context.Context) InFlightTransactionStateGeneration
	GetPreviousGenerations(ctx context.Context) []InFlightTransactionStateGeneration
	NewGeneration(ctx context.Context)
}

type InFlightTransactionStateGeneration interface {
	Cancel(ctx context.Context)
	IsCancelled(ctx context.Context) bool
	SetCurrent(ctx context.Context, current bool)
	IsCurrent(ctx context.Context) bool

	// stage management
	StartNewStageContext(ctx context.Context, stageType InFlightTxStage, substatus BaseTxSubStatus)
	GetStage(ctx context.Context) InFlightTxStage
	SetTransientPreviousStageOutputs(tpso *TransientPreviousStageOutputs)
	GetRunningStageContext(ctx context.Context) *RunningStageContext
	GetStageTriggerError(ctx context.Context) error
	ClearRunningStageContext(ctx context.Context)
	GetStageStartTime(ctx context.Context) time.Time
	SetValidatedTransactionHashMatchState(ctx context.Context, validatedTransactionHashMatchState bool)
	ValidatedTransactionHashMatchState(ctx context.Context) bool

	// stage outputs management
	AddStageOutputs(ctx context.Context, stageOutput *StageOutput)
	ProcessStageOutputs(ctx context.Context, processFunction func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput))
	AddPersistenceOutput(ctx context.Context, stage InFlightTxStage, persistenceTime time.Time, err error)
	AddSubmitOutput(ctx context.Context, txHash *pldtypes.Bytes32, submissionTime *pldtypes.Timestamp, submissionOutcome SubmissionOutcome, errorReason ethclient.ErrorReason, err error)
	AddSignOutput(ctx context.Context, signedMessage []byte, txHash *pldtypes.Bytes32, err error)
	AddGasPriceOutput(ctx context.Context, gasPriceObject *pldapi.PublicTxGasPricing, err error)
	AddPanicOutput(ctx context.Context, stage InFlightTxStage)

	PersistTxState(ctx context.Context) (stage InFlightTxStage, persistenceTime time.Time, err error)
}
