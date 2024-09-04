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

package types

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
)

// BaseTxStatus is the current status of a transaction
type BaseTxStatus string

const (
	// BaseTxStatusPending indicates the operation has been submitted, but is not yet confirmed as successful or failed
	BaseTxStatusPending BaseTxStatus = "Pending"
	// BaseTxStatusSucceeded the infrastructure runtime has returned success for the operation
	BaseTxStatusSucceeded BaseTxStatus = "Succeeded"
	// BaseTxStatusFailed happens when an error is reported by the infrastructure runtime
	BaseTxStatusFailed BaseTxStatus = "Failed"
	// BaseTxStatusSuspended indicates we are not actively doing any work with this transaction right now, until it's resumed to pending again
	BaseTxStatusSuspended BaseTxStatus = "Suspended"
)

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
	// BaseTxActionReceiveReceipt indicates that we have received a receipt for the transaction
	BaseTxActionReceiveReceipt BaseTxAction = "ReceiveReceipt"
	// BaseTxActionConfirmTransaction indicates that the transaction has been confirmed
	BaseTxActionConfirmTransaction BaseTxAction = "Confirm"
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
	Status               *BaseTxStatus        `json:"status"`
	DeleteRequested      *fftypes.FFTime      `json:"deleteRequested,omitempty"`
	From                 *string              `json:"from,omitempty"`
	To                   *string              `json:"to,omitempty"`
	Nonce                *ethtypes.HexInteger `json:"nonce,omitempty"`
	Value                *ethtypes.HexInteger `json:"value,omitempty"`
	GasPrice             *ethtypes.HexInteger `json:"gasPrice,omitempty"`
	MaxPriorityFeePerGas *ethtypes.HexInteger `json:"maxPriorityFeePerGas,omitempty"`
	MaxFeePerGas         *ethtypes.HexInteger `json:"maxFeePerGas,omitempty"`
	GasLimit             *ethtypes.HexInteger `json:"gas,omitempty"` // note this is required for some methods (eth_estimateGas)
	TransactionHash      *string              `json:"transactionHash,omitempty"`
	PolicyInfo           *fftypes.JSONAny     `json:"policyInfo"`
	FirstSubmit          *fftypes.FFTime      `json:"firstSubmit,omitempty"`
	LastSubmit           *fftypes.FFTime      `json:"lastSubmit,omitempty"`
	ErrorMessage         *string              `json:"errorMessage,omitempty"`
}

type TransactionHeaders struct {
	From  string            `json:"from,omitempty"`
	To    string            `json:"to,omitempty"`
	Nonce *fftypes.FFBigInt `json:"nonce,omitempty"`
	Gas   *fftypes.FFBigInt `json:"gas,omitempty"`
	Value *fftypes.FFBigInt `json:"value,omitempty"`
}

type ManagedTX struct {
	ID              string          `json:"id"`
	Created         *fftypes.FFTime `json:"created"`
	Updated         *fftypes.FFTime `json:"updated"`
	Status          BaseTxStatus    `json:"status"`
	DeleteRequested *fftypes.FFTime `json:"deleteRequested,omitempty"`
	SequenceID      string          `json:"sequenceId,omitempty"`
	*ethsigner.Transaction
	TransactionHash string           `json:"transactionHash,omitempty"`
	PolicyInfo      *fftypes.JSONAny `json:"policyInfo"`
	FirstSubmit     *fftypes.FFTime  `json:"firstSubmit,omitempty"`
	LastSubmit      *fftypes.FFTime  `json:"lastSubmit,omitempty"`
	ErrorMessage    string           `json:"errorMessage,omitempty"`
}

type BalanceManager interface {
	TopUpAccount(ctx context.Context, addAccount *AddressAccount) (mtx *ManagedTX, err error)
	IsAutoFuelingEnabled(ctx context.Context) bool
	GetAddressBalance(ctx context.Context, address string) (*AddressAccount, error)
	NotifyAddressBalanceChanged(ctx context.Context, address string)
}

type AutoFuelTransactionHandler interface {
}

// AddressAccount provides the following feature:
// - record a snapshot of the current balance of a signing address
// - record the total spent of a series of transaction emitted by this signing address
// - provide an interface to top up the signing address when spent is higher than the balance
type AddressAccount struct {
	Address               string
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

type NextNonceCallback func(ctx context.Context, signer string) (uint64, error)

type TransactionStore interface {
	GetTransactionByID(ctx context.Context, txID string) (*ManagedTX, error)
	InsertTransactionWithNextNonce(ctx context.Context, tx *ManagedTX, lookupNextNonce NextNonceCallback) error
	UpdateTransaction(ctx context.Context, txID string, updates *BaseTXUpdates) error
	DeleteTransaction(ctx context.Context, txID string) error

	GetTransactionReceipt(ctx context.Context, txID string) (receipt *ethclient.TransactionReceiptResponse, err error)
	SetTransactionReceipt(ctx context.Context, txID string, receipt *ethclient.TransactionReceiptResponse) error

	AddTransactionConfirmations(ctx context.Context, txID string, clearExisting bool, confirmations ...*Confirmation) error
	AddSubStatusAction(ctx context.Context, txID string, subStatus BaseTxSubStatus, action BaseTxAction, info *fftypes.JSONAny, err *fftypes.JSONAny, actionOccurred *fftypes.FFTime) error

	ListTransactions(ctx context.Context, filter ffapi.AndFilter) ([]*ManagedTX, *ffapi.FilterResult, error)
	NewTransactionFilter(ctx context.Context) ffapi.FilterBuilder
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

type BaseLedgerTxEngine interface {
	// Lifecycle functions

	// Init - setting a set of initialized toolkit plugins in the constructed transaction handler object. Safe checks & initialization
	//        can take place inside this function as well. It also enables toolkit plugins to be able to embed a reference to its parent
	//        transaction handler instance.
	Init(ctx context.Context, ethClient ethclient.EthClient, keymgr ethclient.KeyManager, txStore TransactionStore, managedTXEventNotifier ManagedTxEventNotifier, txConfirmationListener TransactionConfirmationListener)

	// Start - starting the transaction handler to handle inbound events.
	// It takes in a context, of which upon cancellation will stop the transaction handler.
	// It returns a read-only channel. When this channel gets closed, it indicates transaction handler has been stopped gracefully.
	// It returns an error when failed to start.
	Start(ctx context.Context) (done <-chan struct{}, err error)

	// Event handling functions
	// Instructional events:
	// HandleNewTransaction - handles event of adding new transactions onto blockchain
	HandleNewTransaction(ctx context.Context, reqOptions *RequestOptions, txPayload interface{}) (mtx *ManagedTX, submissionRejected bool, err error)
	// HandleSuspendTransaction - handles event of suspending a managed transaction
	HandleSuspendTransaction(ctx context.Context, txID string) (mtx *ManagedTX, err error)
	// HandleResumeTransaction - handles event of resuming a suspended managed transaction
	HandleResumeTransaction(ctx context.Context, txID string) (mtx *ManagedTX, err error)

	// Informational events:
	// HandleTransactionConfirmations - handles confirmations of blockchain transactions for a managed transaction
	HandleTransactionConfirmations(ctx context.Context, txID string, notification *ConfirmationsNotification) (err error)
	// HandleTransactionReceiptReceived - handles receipt of blockchain transactions for a managed transaction
	HandleTransactionReceiptReceived(ctx context.Context, txID string, receipt *ethclient.TransactionReceiptResponse) (err error)

	// Functions for auto-fueling
	GetPendingFuelingTransaction(ctx context.Context, sourceAddress string, destinationAddress string) (tx *ManagedTX, err error)
	CheckTransactionCompleted(ctx context.Context, tx *ManagedTX) (completed bool)
}

// Handler checks received transaction process events and dispatch them to an event
// manager accordingly.
type ManagedTxEventNotifier interface {
	Notify(ctx context.Context, e ManagedTransactionEvent) error
}

type TransactionConfirmationListener interface {
	Add(ctx context.Context, txID, txHash string, rH ReceiptHandler, cH ConfirmationHandler) error
	Remove(ctx context.Context, txHash string) error
}

type RequestOptions struct {
	ID       *uuid.UUID
	SignerID string
	GasLimit *ethtypes.HexInteger
}

func (ro *RequestOptions) Validate(ctx context.Context) error {
	if ro.ID == nil {
		return i18n.NewError(ctx, msgs.MsgMissingTransactionID)
	}

	if ro.SignerID == "" {
		return i18n.NewError(ctx, msgs.MsgErrorMissingSignerID)
	}
	return nil
}

// ManagedTransactionEventType is a enum type that contains all types of transaction process events
// that a transaction handler emits.
type ManagedTransactionEventType int

const (
	ManagedTXProcessSucceeded ManagedTransactionEventType = iota
	ManagedTXProcessFailed
)

type ManagedTransactionEvent struct {
	Type    ManagedTransactionEventType
	Tx      *ManagedTX
	Receipt *ethclient.TransactionReceiptResponse
}

// ReceiptHandler can be passed on the event as a closure with extra variables
type ReceiptHandler func(ctx context.Context, txID string, receipt *ethclient.TransactionReceiptResponse) error

// ConfirmationHandler can be passed on the event as a closure with extra variables
type ConfirmationHandler func(ctx context.Context, txID string, notification *ConfirmationsNotification) error

type GasPriceObject struct {
	MaxPriorityFeePerGas *big.Int `json:"maxPriorityFeePerGas,omitempty"`
	MaxFeePerGas         *big.Int `json:"maxFeePerGas,omitempty"`
	GasPrice             *big.Int `json:"gasPrice,omitempty"`
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
	//   entry criteria (OR):
	//     received "receipt" (!!!first high priority, before entry criteria checks of any other stages, other stage are aborted as soon as this criteria is met)
	//     the last sub-status is a success "submit" substatus
	//     the last sub-status is an unexpired incomplete "receipt" substatus
	//   emitted async actions:
	//     - old transaction dis-tracking (only emitted for expired )
	//     - new transaction tracking
	//   completion criteria -> confirm / prepare
	//     the last sub-status is a completed (success/expired) "receipt" substatus
	InFlightTxStageReceipting InFlightTxStage = "receipt"
	//   entry criteria:
	//     there is a completed "receipt" substatus (!!!second high priority)
	//   completion criteria -> complete
	//      the last sub-status is a completed "confirmed" substatus
	InFlightTxStageConfirming InFlightTxStage = "confirm"
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
	string(InFlightTxStageReceipting),
	string(InFlightTxStageConfirming),
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

type EnterprisePolicyInfo struct {
	LastWarnTime      *fftypes.FFTime `json:"lastWarnTime"`
	SubmittedTxHashes []string        `json:"submittedTxHashes,omitempty"`
}

type InMemoryTxStateReadOnly interface {
	GetTxID() string
	GetCreatedTime() *fftypes.FFTime
	GetDeleteRequestedTime() *fftypes.FFTime
	// get the transaction receipt from the in-memory state (note: the returned value should not be modified)
	GetReceipt() *ethclient.TransactionReceiptResponse
	GetTransactionHash() string
	GetNonce() *big.Int
	GetFrom() string
	GetStatus() BaseTxStatus
	GetGasPriceObject() *GasPriceObject
	GetFirstSubmit() *fftypes.FFTime
	GetPolicyInfo() *EnterprisePolicyInfo

	GetTx() *ManagedTX //TODO: remove the need of this function

	GetGasLimit() *big.Int
	IsComplete() bool
	IsSuspended() bool
}
type InMemoryTxStateManager interface {
	InMemoryTxStateReadOnly
	InMemoryTxStateSetters
}

type InMemoryTxStateSetters interface {
	SetReceipt(ctx context.Context, receipt *ethclient.TransactionReceiptResponse)
	ApplyTxUpdates(ctx context.Context, txUpdates *BaseTXUpdates)
}

type StageOutput struct {
	Stage InFlightTxStage

	PersistenceOutput *PersistenceOutput

	SubmitOutput *SubmitOutputs

	SignOutput *SignOutputs

	GasPriceOutput *GasPriceOutput

	ReceiptOutput *ReceiptOutputs

	ConfirmationOutput *ConfirmationOutputs
}

type SubmitOutputs struct {
	TxHash            string
	SubmissionTime    *fftypes.FFTime
	SubmissionOutcome SubmissionOutcome
	ErrorReason       string
	Err               error
}
type SignOutputs struct {
	SignedMessage []byte
	TxHash        string
	Err           error
}

type GasPriceOutput struct {
	GasPriceObject *GasPriceObject
	Err            error
}

type ReceiptOutputs struct {
	Receipt       *ethclient.TransactionReceiptResponse
	ReceiptNotify *fftypes.FFTime
	Err           error
}

type ConfirmationOutputs struct {
	Confirmations *ConfirmationsNotification
	ConfirmNotify *fftypes.FFTime
}

type PersistenceOutput struct {
	PersistenceError error
	Time             time.Time
}

type InFlightStageActionTriggers interface {
	TriggerRetrieveGasPrice(ctx context.Context) error
	TriggerTracking(ctx context.Context) error
	TriggerSignTx(ctx context.Context) error
	TriggerSubmitTx(ctx context.Context, signedMessage []byte) error
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

func (ctx *RunningStageContext) SetNewPersistenceUpdateOutput() {
	if ctx.StageOutputsToBePersisted == nil {
		ctx.StageOutputsToBePersisted = &RunningStageContextPersistenceOutput{
			UpdateType: PersistenceUpdateUpdate,
			InMemoryTx: ctx.InMemoryTx,
			SubStatus:  ctx.SubStatus,
			Ctx:        ctx,
		}
	}
}

type RunningStageContextPersistenceOutput struct {
	UpdateType PersistenceUpdateType
	InMemoryTx InMemoryTxStateReadOnly
	SubStatus  BaseTxSubStatus
	Ctx        context.Context

	PolicyInfo     *EnterprisePolicyInfo
	TxUpdates      *BaseTXUpdates
	Receipt        *ethclient.TransactionReceiptResponse
	HistoryUpdates []func(p TransactionStore) error
	Confirmations  *ConfirmationsNotification
}

func (sOut *RunningStageContextPersistenceOutput) AddSubStatusAction(action BaseTxAction, info *fftypes.JSONAny, err *fftypes.JSONAny) {
	actionOccurred := fftypes.Now()
	sOut.HistoryUpdates = append(sOut.HistoryUpdates, func(p TransactionStore) error {
		return p.AddSubStatusAction(sOut.Ctx, sOut.InMemoryTx.GetTxID(), sOut.SubStatus, action, info, err, actionOccurred)
	})
}

// UpdateType informs FFTM whether the transaction needs an update to be persisted after this execution of the policy engine
type PersistenceUpdateType int

const (
	PersistenceUpdateUpdate PersistenceUpdateType = iota // Instructs that the transaction should be updated in persistence
	// persistenceUpdateDelete                              // Instructs that the transaction should be removed completely from persistence - not sure it's safe to do so
)

type OrchestratorContext struct {
	// input from transaction engine
	AvailableToSpend         *big.Int
	PreviousNonceCostUnknown bool
}

// output of some stages doesn't get written into the database
// so it needs to be carried over to next stages
type TransientPreviousStageOutputs struct {
	SignedMessage []byte // NB: if the value is nil when triggering submitTx , node signer will be used to sign the transaction instead, don't use this to judge whether a transaction can be submitted or not.
}

type InFlightTransactionStateManager interface {
	// tx state management
	InMemoryTxStateReadOnly
	CanSubmit(ctx context.Context, cost *big.Int) bool
	CanBeRemoved(ctx context.Context) bool

	// stage management
	StartNewStageContext(ctx context.Context, stage InFlightTxStage, substatus BaseTxSubStatus)
	GetStage(ctx context.Context) InFlightTxStage
	SetOrchestratorContext(ctx context.Context, tec *OrchestratorContext)
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
	AddSubmitOutput(ctx context.Context, txHash string, submissionTime *fftypes.FFTime, submissionOutcome SubmissionOutcome, errorReason ethclient.ErrorReason, err error)
	AddSignOutput(ctx context.Context, signedMessage []byte, txHash string, err error)
	AddGasPriceOutput(ctx context.Context, gasPriceObject *GasPriceObject, err error)
	AddReceiptOutput(ctx context.Context, rpt *ethclient.TransactionReceiptResponse, err error)
	AddConfirmationsOutput(ctx context.Context, cmfs *ConfirmationsNotification)
	AddPanicOutput(ctx context.Context, stage InFlightTxStage)

	PersistTxState(ctx context.Context) (stage InFlightTxStage, persistenceTime time.Time, err error)
}
