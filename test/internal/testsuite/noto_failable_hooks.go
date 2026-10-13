// Copyright © 2026 Kaleido, Inc.
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

package testsuite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/test/internal/conf"
	"github.com/LFDT-Paladin/paladin/test/internal/contracts"
	"github.com/LFDT-Paladin/paladin/test/internal/util"

	nototypes "github.com/LFDT-Paladin/paladin/test/internal/contracts"
	"github.com/hyperledger-firefly/signer/pkg/abi"
	log "github.com/sirupsen/logrus"
)

const notoRevertableHooksDefaultListenerName = "notorevertablehookslistener"
const notoRevertableHooksDefaultErrorInterval = "15s"
const notoRevertableHooksDefaultInitialMintAmount = int64(1000000)
const notoRevertableHooksDefaultIncludeInvalidInputErrors = true

const notoResolveAlgorithm = "ecdsa:secp256k1"
const notoResolveVerifierType = "eth_address"

var notoConstructorABI = contracts.NotoConstructorABI

type TransferAction int

const (
	notoActionRevert TransferAction = iota
	notoActionFail
	notoActionInvalidInput
	notoActionSuccess
)

// pendingEntry carries a completed txID together with the outcome that was
// expected for it at submission time.
type pendingEntry struct {
	txID   string
	action TransferAction
}

type notoRevertableHooksSuite struct {
	ctx                  context.Context
	runner               Runner
	notoContractAddress  *pldtypes.EthAddress
	revertableAddress    *pldtypes.EthAddress
	notary               string
	sender               string
	revertIdentity       string
	failIdentity         string
	invalidInputIdentity string
	successIdentity      string
	revertAddress        *pldtypes.EthAddress
	failAddress          *pldtypes.EthAddress
	invalidInputAddress  *pldtypes.EthAddress
	successAddress       *pldtypes.EthAddress
	listenerName         string
	initialMintAmount    int64
	errorInterval        time.Duration
	includeInvalidInput  bool
	errorQueue           chan TransferAction
	tickerStop           chan struct{}
	tickerWG             sync.WaitGroup
	sub                  rpcclient.Subscription

	// pendingOutcome maps txID → expected TransferAction. Entries are added at
	// submission time and deleted when OnReceiptBatch moves them to checkBatch.
	// The map is bounded to "submitted but not yet completed".
	pendingOutcome sync.Map

	resultsMu     sync.Mutex
	failures      []string
	informational map[string][]string
}

type notoRevertableHooksOptions struct {
	ListenerName              *string `json:"listenerName"`
	InitialMintAmount         *int64  `json:"initialMintAmount"`
	ErrorInterval             *string `json:"errorInterval"`
	IncludeInvalidInputErrors *bool   `json:"includeInvalidInputErrors"`
}

func NewNotoRevertableHooksSuite(ctx context.Context, runner Runner) *notoRevertableHooksSuite {
	return &notoRevertableHooksSuite{ctx: ctx, runner: runner}
}

func (s *notoRevertableHooksSuite) parseNotoRevertableHooksOptions(options map[string]any) error {
	var input notoRevertableHooksOptions
	if options != nil {
		raw, err := json.Marshal(options)
		if err != nil {
			return fmt.Errorf("invalid test.options for noto_revertable_hooks: %w", err)
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return fmt.Errorf("unable to parse test.options for noto_revertable_hooks: %w", err)
		}
	}

	s.listenerName = confutil.StringNotEmpty(input.ListenerName, notoRevertableHooksDefaultListenerName)
	s.initialMintAmount = confutil.Int64Min(input.InitialMintAmount, 1, notoRevertableHooksDefaultInitialMintAmount)
	s.errorInterval = confutil.DurationMin(input.ErrorInterval, time.Millisecond, notoRevertableHooksDefaultErrorInterval)
	s.includeInvalidInput = confutil.Bool(input.IncludeInvalidInputErrors, notoRevertableHooksDefaultIncludeInvalidInputErrors)
	return nil
}

func (s *notoRevertableHooksSuite) startErrorTicker(interval time.Duration) {
	s.tickerWG.Add(1)
	go func() {
		defer s.tickerWG.Done()
		timer := time.NewTimer(interval)
		defer timer.Stop()
		rotation := []TransferAction{
			notoActionRevert,
			notoActionFail,
		}
		if s.includeInvalidInput {
			rotation = append(rotation, notoActionInvalidInput)
		}
		nextAction := 0
		for {
			select {
			case <-s.tickerStop:
				return
			case <-timer.C:
				select {
				case <-s.tickerStop:
					return
				case s.errorQueue <- rotation[nextAction]:
					nextAction = (nextAction + 1) % len(rotation)
					timer.Reset(interval)
				}
			}
		}
	}()
}

func (s *notoRevertableHooksSuite) trackTransaction(actionType TransferAction, txID string) {
	s.pendingOutcome.Store(txID, actionType)
}

func (s *notoRevertableHooksSuite) Setup() error {
	nodes := s.runner.GetNodes()
	if len(nodes) != 3 {
		return fmt.Errorf("noto_revertable_hooks requires exactly 3 nodes, got %d", len(nodes))
	}

	node1 := nodes[0]
	node2 := nodes[1]
	node3 := nodes[2]
	testCfg := s.runner.GetTestConfig()
	if err := s.parseNotoRevertableHooksOptions(testCfg.Options); err != nil {
		return err
	}

	s.informational = make(map[string][]string)

	s.notary = fmt.Sprintf("member@%s", node3.Config.Name)
	s.sender = fmt.Sprintf("member@%s", node2.Config.Name)
	s.revertIdentity = fmt.Sprintf("revert@%s", node1.Config.Name)
	s.failIdentity = fmt.Sprintf("fail@%s", node3.Config.Name)
	s.invalidInputIdentity = fmt.Sprintf("invalidinput@%s", node3.Config.Name)
	s.successIdentity = fmt.Sprintf("succeed@%s", node2.Config.Name)

	resolveAddr := func(identity string) (*pldtypes.EthAddress, error) {
		verifier, err := node2.HTTPClient.PTX().ResolveVerifier(
			s.ctx,
			identity,
			notoResolveAlgorithm,
			notoResolveVerifierType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve verifier for %s: %w", identity, err)
		}
		addr := pldtypes.MustEthAddress(verifier)
		return addr, nil
	}
	var err error
	s.revertAddress, err = resolveAddr(s.revertIdentity)
	if err != nil {
		return err
	}
	s.failAddress, err = resolveAddr(s.failIdentity)
	if err != nil {
		return err
	}
	s.invalidInputAddress, err = resolveAddr(s.invalidInputIdentity)
	if err != nil {
		return err
	}
	s.successAddress, err = resolveAddr(s.successIdentity)
	if err != nil {
		return err
	}
	if s.includeInvalidInput {
		log.Infof(
			"Resolved action addresses: revert=%s (%s) fail=%s (%s) invalid_input=%s (%s) success=%s (%s)",
			s.revertIdentity, s.revertAddress.String(),
			s.failIdentity, s.failAddress.String(),
			s.invalidInputIdentity, s.invalidInputAddress.String(),
			s.successIdentity, s.successAddress.String(),
		)
	} else {
		log.Infof(
			"Resolved action addresses: revert=%s (%s) fail=%s (%s) invalid_input=%s (%s) success=%s (%s) (invalid_input disabled in rotation)",
			s.revertIdentity, s.revertAddress.String(),
			s.failIdentity, s.failAddress.String(),
			s.invalidInputIdentity, s.invalidInputAddress.String(),
			s.successIdentity, s.successAddress.String(),
		)
	}

	// --- Step 1: Deploy RevertableTarget on the base ledger ---
	log.Info("Loading RevertableTarget contract...")
	revertableTarget, err := contracts.LoadRevertableTargetContract()
	if err != nil {
		return err
	}
	log.Info("Deploying RevertableTarget as public transaction...")
	revertableDeployTxID, err := nodes[0].HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "deploy",
			Data: pldtypes.RawJSON(`[]`),
		},
		ABI:      revertableTarget.ABI,
		Bytecode: revertableTarget.Bytecode,
	})
	if err != nil {
		return fmt.Errorf("failed to deploy RevertableTarget: %w", err)
	}

	log.Info("Waiting for RevertableTarget deployment receipt...")
	revertableReceipt, err := util.WaitForTransactionReceiptFull(s.ctx, nodes[0].HTTPClient, *revertableDeployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get RevertableTarget deployment receipt: %w", err)
	}
	if !revertableReceipt.Success {
		return fmt.Errorf("RevertableTarget deployment failed: %s", revertableReceipt.FailureMessage)
	}
	if revertableReceipt.ContractAddress == nil {
		return fmt.Errorf("RevertableTarget contract address not found in deployment receipt")
	}
	s.revertableAddress = revertableReceipt.ContractAddress
	log.Infof("RevertableTarget deployed at address: %s", *s.revertableAddress)

	// --- Step 2: Create Pente privacy group ---
	members := []string{
		fmt.Sprintf("member@%s", node1.Config.Name),
		fmt.Sprintf("member@%s", node3.Config.Name),
	}

	log.Info("Creating Pente privacy group...")
	group, err := nodes[0].HTTPClient.PrivacyGroups().CreateGroup(s.ctx, &pldapi.PrivacyGroupInput{
		Domain:  "pente",
		Members: members,
		Name:    "noto-revertable-hooks-test",
		Configuration: map[string]string{
			"evmVersion":           "shanghai",
			"externalCallsEnabled": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create privacy group: %w", err)
	}
	log.Infof("Privacy group created with ID: %s", group.ID)

	log.Info("Waiting for privacy group genesis receipt...")
	groupReceipt, err := util.WaitForTransactionReceipt(s.ctx, nodes[0].HTTPClient, group.GenesisTransaction, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get privacy group genesis receipt: %w", err)
	}
	if !groupReceipt.Success {
		return fmt.Errorf("privacy group creation failed: %s", groupReceipt.FailureMessage)
	}
	penteAddress := groupReceipt.ContractAddress
	log.Infof("Privacy group contract address: %s", penteAddress)

	// --- Step 3: Deploy NotoHooksRevertable inside the privacy group ---
	log.Info("Loading NotoHooksRevertable contract...")
	notoHooks, err := contracts.LoadNotoHooksRevertableContract()
	if err != nil {
		return err
	}

	var hooksConstructor *abi.Entry
	for _, entry := range notoHooks.ABI {
		if entry.Type == abi.Constructor {
			hooksConstructor = entry
			break
		}
	}

	log.Info("Deploying NotoHooksRevertable to privacy group...")
	hooksDeployTxID, err := nodes[0].HTTPClient.PrivacyGroups().SendTransaction(s.ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  group.ID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			Bytecode: notoHooks.Bytecode,
			Function: hooksConstructor,
			Input: pldtypes.RawJSON(fmt.Sprintf(
				`{"_revertableTarget":"%s","_revertAddress":"%s","_failAddress":"%s","_invalidInputAddress":"%s"}`,
				s.revertableAddress,
				s.revertAddress,
				s.failAddress,
				s.invalidInputAddress,
			)),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to deploy NotoHooksRevertable: %w", err)
	}

	log.Info("Waiting for NotoHooksRevertable deployment receipt...")
	hooksReceipt, err := util.WaitForTransactionReceiptFull(s.ctx, nodes[0].HTTPClient, hooksDeployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get NotoHooksRevertable deployment receipt: %w", err)
	}
	if !hooksReceipt.Success {
		return fmt.Errorf("NotoHooksRevertable deployment failed: %s", hooksReceipt.FailureMessage)
	}

	var hooksPrivateAddress *pldtypes.EthAddress
	if hooksReceipt.DomainReceipt != nil {
		var domainReceipt pldapi.PenteDomainReceipt
		if err := json.Unmarshal(hooksReceipt.DomainReceipt, &domainReceipt); err == nil {
			if domainReceipt.Receipt != nil && domainReceipt.Receipt.ContractAddress != nil {
				hooksPrivateAddress = domainReceipt.Receipt.ContractAddress
			}
		}
	}
	if hooksPrivateAddress == nil {
		return fmt.Errorf("NotoHooksRevertable private contract address not found in domain receipt")
	}
	log.Infof("NotoHooksRevertable deployed at private address: %s", *hooksPrivateAddress)

	// --- Step 4: Deploy Noto domain instance with hooks ---
	log.Info("Deploying Noto with revertable hooks...")
	constructorParams := map[string]any{
		"notary":     s.notary,
		"notaryMode": string(nototypes.NotaryModeHooks),
		"options": map[string]any{
			"hooks": map[string]any{
				"publicAddress":  penteAddress.String(),
				"privateAddress": hooksPrivateAddress.String(),
				"privateGroup": map[string]any{
					"salt":    group.ID.String(),
					"members": members,
				},
			},
		},
	}

	notoDeployResult := node3.HTTPClient.ForABI(s.ctx, notoConstructorABI).
		Private().
		Domain("noto").
		Constructor().
		From("member").
		Inputs(constructorParams).
		Send().
		Wait(60 * time.Second)
	if notoDeployResult.Error() != nil {
		return fmt.Errorf("failed to deploy Noto: %w", notoDeployResult.Error())
	}
	if notoDeployResult.Receipt().ContractAddress == nil {
		return fmt.Errorf("Noto contract address not found in deployment receipt")
	}
	s.notoContractAddress = notoDeployResult.Receipt().ContractAddress
	log.Infof("Noto deployed at address: %s", *s.notoContractAddress)

	// --- Step 5: Mint initial supply to sender ---
	log.Info("Minting initial supply to sender...")
	mintParams := &nototypes.MintParams{
		To:     s.sender,
		Amount: pldtypes.Uint64ToUint256(uint64(s.initialMintAmount)),
	}
	mintJSON, err := json.Marshal(mintParams)
	if err != nil {
		return fmt.Errorf("failed to marshal initial mint params: %w", err)
	}
	mintTxID, err := node3.HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Domain:   "noto",
			Function: "mint",
			To:       s.notoContractAddress,
			From:     "member",
			Data:     pldtypes.RawJSON(mintJSON),
		},
		ABI: nototypes.NotoABI,
	})
	if err != nil {
		return fmt.Errorf("failed to send initial mint transaction: %w", err)
	}
	mintReceipt, err := util.WaitForTransactionReceipt(s.ctx, node3.HTTPClient, *mintTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get initial mint receipt: %w", err)
	}
	if !mintReceipt.Success {
		return fmt.Errorf("initial mint failed: %s", mintReceipt.FailureMessage)
	}
	log.Infof("Initial supply of %d minted to %s", s.initialMintAmount, s.sender)

	// --- Step 6: Create receipt listener for Noto on node2 ---
	listenerNode := node2
	var latestSequence *uint64
	qb := query.NewQueryBuilder().Equal("domain", "noto").Sort("-sequence").Limit(1)
	receipts, err := listenerNode.HTTPClient.PTX().QueryTransactionReceipts(s.ctx, qb.Query())
	if err == nil && len(receipts) > 0 {
		seq := receipts[0].Sequence
		latestSequence = &seq
		log.Infof("Found latest sequence: %d, will start listener from sequence above this", seq)
	} else {
		log.Info("No existing Noto receipts found, starting listener from beginning")
	}

	_, _ = listenerNode.HTTPClient.PTX().DeleteReceiptListener(s.ctx, s.listenerName)

	txType := pldapi.TransactionTypePrivate.Enum()
	_, err = listenerNode.HTTPClient.PTX().CreateReceiptListener(s.ctx, &pldapi.TransactionReceiptListener{
		Name: s.listenerName,
		Filters: pldapi.TransactionReceiptFilters{
			Type:          &txType,
			Domain:        "noto",
			SequenceAbove: latestSequence,
		},
		Options: pldapi.TransactionReceiptListenerOptions{
			DomainReceipts:                 true,
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorProcess.Enum(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create receipt listener: %w", err)
	}

	// do this last so we're not starting the test with an error already on the queue
	s.errorQueue = make(chan TransferAction, 1)
	s.tickerStop = make(chan struct{})
	s.startErrorTicker(s.errorInterval)

	log.Info("Noto revertable hooks test setup complete")
	return nil
}

func (s *notoRevertableHooksSuite) Subscribe() (rpcclient.Subscription, error) {
	nodes := s.runner.GetNodes()
	if len(nodes) < 2 {
		return nil, fmt.Errorf("noto_revertable_hooks requires at least 2 nodes for receipt subscription")
	}
	sub, err := nodes[1].WSClient.PTX().SubscribeReceipts(s.ctx, s.listenerName)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to noto receipts: %w", err)
	}
	s.sub = sub
	return sub, nil
}

// OnReceiptBatch is called by the runner every N completions. It resolves the expected
// outcome for each txID from pendingOutcome, validates the receipts, and records failures.
// Concurrent calls are safe via resultsMu and sync.Map.
func (s *notoRevertableHooksSuite) OnReceiptBatch(txIDs []string) {
	entries := make([]pendingEntry, 0, len(txIDs))
	for _, id := range txIDs {
		v, ok := s.pendingOutcome.LoadAndDelete(id)
		if !ok {
			continue
		}
		entries = append(entries, pendingEntry{txID: id, action: v.(TransferAction)})
	}
	if len(entries) > 0 {
		s.checkBatch(entries)
	}
}

func (s *notoRevertableHooksSuite) checkBatch(entries []pendingEntry) {
	nodes := s.runner.GetNodes()
	if len(nodes) < 2 {
		return
	}
	submitterClient := nodes[1].HTTPClient

	const failRevertReason = "Configured to fail"
	const revertRevertReason = "Configured to revert"
	const notoInvalidInputSelector = "8b8ff76e"
	const penteInputNotAvailableReason = "PenteInputNotAvailable"
	const dependencyFailedReason = "PD012256"
	failRevertReasonHex := fmt.Sprintf("%x", failRevertReason)
	revertRevertReasonHex := fmt.Sprintf("%x", revertRevertReason)

	ids := make([]any, len(entries))
	for i, e := range entries {
		ids[i] = e.txID
	}
	txsFull, err := submitterClient.PTX().QueryTransactionsFull(
		s.ctx,
		query.NewQueryBuilder().In("id", ids).Limit(len(entries)).Query(),
	)
	if err != nil {
		s.resultsMu.Lock()
		s.failures = append(s.failures, fmt.Sprintf("failed to query transactions for batch of %d: %v", len(entries), err))
		s.resultsMu.Unlock()
		return
	}

	txByID := make(map[string]*pldapi.TransactionFull, len(txsFull))
	for _, tx := range txsFull {
		if tx.Transaction != nil {
			txByID[tx.Transaction.ID.String()] = tx
		}
	}

	countDispatches := func(activity []*pldapi.SequencerActivity) int {
		dispatches := 0
		for _, a := range activity {
			if a == nil {
				continue
			}
			if strings.Contains(strings.ToLower(a.ActivityType), "dispatch") {
				dispatches++
			}
		}
		return dispatches
	}

	var batchFailures []string
	var penteInputNotAvailableTxIDs []string
	var dependencyFailedTxIDs []string

	for _, entry := range entries {
		tx := txByID[entry.txID]
		if tx == nil {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s not found in query results", entry.txID))
			continue
		}
		receipt := tx.Receipt
		if receipt == nil {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s has no receipt", entry.txID))
			continue
		}

		// Informational outcomes — not failures.
		if !receipt.Success && strings.Contains(receipt.FailureMessage, penteInputNotAvailableReason) {
			penteInputNotAvailableTxIDs = append(penteInputNotAvailableTxIDs, entry.txID)
			continue
		}
		if !receipt.Success && strings.HasPrefix(receipt.FailureMessage, dependencyFailedReason) {
			dependencyFailedTxIDs = append(dependencyFailedTxIDs, entry.txID)
			continue
		}

		switch entry.action {
		case notoActionSuccess:
			if !receipt.Success {
				batchFailures = append(batchFailures, fmt.Sprintf("SUCCESS tx %s expected success but failed: %s", entry.txID, receipt.FailureMessage))
			}
		case notoActionFail:
			if receipt.Success {
				batchFailures = append(batchFailures, fmt.Sprintf("FAIL tx %s expected failure but succeeded", entry.txID))
			} else if !strings.Contains(receipt.FailureMessage, failRevertReasonHex) &&
				!strings.Contains(receipt.FailureMessage, failRevertReason) {
				batchFailures = append(batchFailures, fmt.Sprintf("FAIL tx %s failed with unexpected reason: %s", entry.txID, receipt.FailureMessage))
			}
		case notoActionRevert:
			if receipt.Success {
				batchFailures = append(batchFailures, fmt.Sprintf("REVERT tx %s expected failure but succeeded", entry.txID))
			} else if !strings.Contains(receipt.FailureMessage, revertRevertReasonHex) &&
				!strings.Contains(receipt.FailureMessage, revertRevertReason) {
				batchFailures = append(batchFailures, fmt.Sprintf("REVERT tx %s failed with unexpected reason: %s", entry.txID, receipt.FailureMessage))
			}
		case notoActionInvalidInput:
			if receipt.Success {
				batchFailures = append(batchFailures, fmt.Sprintf("INVALID_INPUT tx %s expected failure but succeeded", entry.txID))
			} else if !strings.Contains(receipt.FailureMessage, notoInvalidInputSelector) {
				batchFailures = append(batchFailures, fmt.Sprintf("INVALID_INPUT tx %s failed with unexpected reason: %s", entry.txID, receipt.FailureMessage))
			} else {
				dispatches := countDispatches(tx.SequencerActivity)
				if dispatches != 4 {
					batchFailures = append(batchFailures, fmt.Sprintf("INVALID_INPUT tx %s had %d dispatches in sequencer activity, expected 4", entry.txID, dispatches))
				}
			}
		}
	}

	log.Infof("notoRevertableHooksSuite: rolling check batch=%d failures=%d pente_input_n/a=%d dep_failed=%d",
		len(entries), len(batchFailures), len(penteInputNotAvailableTxIDs), len(dependencyFailedTxIDs))

	if len(batchFailures) > 0 {
		for _, f := range batchFailures {
			log.Errorf("notoRevertableHooksSuite: rolling check failure: %s", f)
		}
	}

	s.resultsMu.Lock()
	s.failures = append(s.failures, batchFailures...)
	if len(penteInputNotAvailableTxIDs) > 0 {
		s.informational[penteInputNotAvailableReason] = append(s.informational[penteInputNotAvailableReason], penteInputNotAvailableTxIDs...)
	}
	if len(dependencyFailedTxIDs) > 0 {
		s.informational[dependencyFailedReason] = append(s.informational[dependencyFailedReason], dependencyFailedTxIDs...)
	}
	s.resultsMu.Unlock()
}

func (s *notoRevertableHooksSuite) Unsubscribe() {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(s.ctx); err != nil {
			log.Errorf("Error unsubscribing from subscription: %s", err.Error())
		} else {
			log.Info("Successfully unsubscribed")
		}
		s.sub = nil
	}
}

func (s *notoRevertableHooksSuite) Cleanup() {
	if s.tickerStop != nil {
		close(s.tickerStop)
		s.tickerWG.Wait()
		s.tickerStop = nil
	}
	nodes := s.runner.GetNodes()
	if len(nodes) > 1 {
		_, err := nodes[1].HTTPClient.PTX().DeleteReceiptListener(s.ctx, s.listenerName)
		if err != nil {
			log.Debugf("Failed to delete receipt listener %s: %v", s.listenerName, err)
		} else {
			log.Infof("Successfully deleted receipt listener: %s", s.listenerName)
		}
	}
}

func (s *notoRevertableHooksSuite) NewWorker(startTime int64, workerID int) TestCase {
	return &notoRevertableHooksWorker{
		testBase: testBase{
			ctx:       s.ctx,
			startTime: startTime,
			workerID:  workerID,
		},
		suite:                s,
		notoContractAddress:  s.notoContractAddress,
		notary:               s.notary,
		sender:               s.sender,
		revertIdentity:       s.revertIdentity,
		failIdentity:         s.failIdentity,
		invalidInputIdentity: s.invalidInputIdentity,
		successIdentity:      s.successIdentity,
		runner:               s.runner,
	}
}

func (s *notoRevertableHooksSuite) PostRun() error {
	s.resultsMu.Lock()
	defer s.resultsMu.Unlock()

	const penteInputNotAvailableReason = "PenteInputNotAvailable"
	const dependencyFailedReason = "PD012256"

	// A small number of transactions can exhaust their retry limit after repeatedly being queued
	// behind a chained dependency that fails. This is not possible to eliminate entirely, so it
	// is treated as informational rather than a test failure.
	if ids := s.informational[penteInputNotAvailableReason]; len(ids) > 0 {
		log.Infof(
			"Informational: %d transactions hit %s: %s",
			len(ids),
			penteInputNotAvailableReason,
			strings.Join(ids, ", "),
		)
	}
	if ids := s.informational[dependencyFailedReason]; len(ids) > 0 {
		log.Infof(
			"Informational: %d transactions hit %s (chained dependency failed): %s",
			len(ids),
			dependencyFailedReason,
			strings.Join(ids, ", "),
		)
	}

	log.Infof(
		"notoRevertableHooksSuite: post-run complete, failures=%d informational_pente_input=%d informational_dep_failed=%d",
		len(s.failures),
		len(s.informational[penteInputNotAvailableReason]),
		len(s.informational[dependencyFailedReason]),
	)

	if len(s.failures) > 0 {
		return fmt.Errorf("rolling check failures (%d): %s", len(s.failures), strings.Join(s.failures, "; "))
	}
	return nil
}

type notoRevertableHooksWorker struct {
	testBase
	suite                *notoRevertableHooksSuite
	notoContractAddress  *pldtypes.EthAddress
	notary               string
	sender               string
	revertIdentity       string
	failIdentity         string
	invalidInputIdentity string
	successIdentity      string
	runner               Runner
}

func (tc *notoRevertableHooksWorker) Name() conf.TestName {
	return conf.PerfTestNotoRevertableHooks
}

func (tc *notoRevertableHooksWorker) RunOnce(iterationCount int) (string, error) {
	nodes := tc.runner.GetNodes()
	if len(nodes) < 2 {
		return "", fmt.Errorf("noto_revertable_hooks requires at least 2 nodes for worker submissions")
	}

	actionType := notoActionSuccess
	select {
	case actionType = <-tc.suite.errorQueue:
	default:
	}
	var targetIdentity string
	switch actionType {
	case notoActionRevert:
		targetIdentity = tc.revertIdentity
	case notoActionInvalidInput:
		targetIdentity = tc.invalidInputIdentity
	case notoActionFail:
		targetIdentity = tc.failIdentity
	default:
		targetIdentity = tc.successIdentity
	}
	log.Debugf("Sending %v transaction", actionType)

	transferParams := &nototypes.TransferParams{
		To:     targetIdentity,
		Amount: pldtypes.Uint64ToUint256(1),
	}
	transferJSON, err := json.Marshal(transferParams)
	if err != nil {
		return "", fmt.Errorf("failed to marshal transfer params: %w", err)
	}

	txID, err := nodes[1].HTTPClient.PTX().SendTransaction(tc.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "noto",
			Function:       "transfer",
			To:             tc.notoContractAddress,
			From:           tc.sender,
			Data:           pldtypes.RawJSON(transferJSON),
			IdempotencyKey: util.GetIdempotencyKey(tc.startTime, tc.workerID, iterationCount),
		},
		ABI: nototypes.NotoABI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send noto transfer transaction: %w", err)
	}
	tc.suite.trackTransaction(actionType, txID.String())

	return txID.String(), nil
}
