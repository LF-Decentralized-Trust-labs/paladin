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

const notoPenteTrackerDefaultListenerName = "notopentetrackerlistener"
const notoPenteTrackerDefaultInitialMintAmount = int64(1000000)

type notoPenteTrackerSuite struct {
	ctx                 context.Context
	runner              Runner
	notoContractAddress *pldtypes.EthAddress
	notary              string
	members             []string
	listenerName        string
	initialMintAmount   int64
	sub                 rpcclient.Subscription

	resultsMu sync.Mutex
	failures  []string
}

type notoPenteTrackerOptions struct {
	ListenerName      *string `json:"listenerName"`
	InitialMintAmount *int64  `json:"initialMintAmount"`
}

func NewNotoPenteTrackerSuite(ctx context.Context, runner Runner) *notoPenteTrackerSuite {
	return &notoPenteTrackerSuite{ctx: ctx, runner: runner}
}

func (s *notoPenteTrackerSuite) parseOptions(options map[string]any) error {
	var input notoPenteTrackerOptions
	if options != nil {
		raw, err := json.Marshal(options)
		if err != nil {
			return fmt.Errorf("invalid test.options for noto_pente_tracker: %w", err)
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return fmt.Errorf("unable to parse test.options for noto_pente_tracker: %w", err)
		}
	}
	s.listenerName = confutil.StringNotEmpty(input.ListenerName, notoPenteTrackerDefaultListenerName)
	s.initialMintAmount = confutil.Int64Min(input.InitialMintAmount, 1, notoPenteTrackerDefaultInitialMintAmount)
	return nil
}

func (s *notoPenteTrackerSuite) Setup() error {
	nodes := s.runner.GetNodes()
	if len(nodes) < 1 {
		return fmt.Errorf("noto_pente_tracker requires at least 1 node")
	}

	testCfg := s.runner.GetTestConfig()
	if err := s.parseOptions(testCfg.Options); err != nil {
		return err
	}

	notaryNode := nodes[0]
	s.notary = fmt.Sprintf("member@%s", notaryNode.Config.Name)
	s.members = make([]string, len(nodes))
	for i, n := range nodes {
		s.members[i] = fmt.Sprintf("member@%s", n.Config.Name)
	}

	// --- Step 1: Create Pente privacy group ---
	log.Info("Creating Pente privacy group...")
	group, err := notaryNode.HTTPClient.PrivacyGroups().CreateGroup(s.ctx, &pldapi.PrivacyGroupInput{
		Domain:  "pente",
		Members: s.members,
		Name:    "noto-pente-tracker-test",
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
	groupReceipt, err := util.WaitForTransactionReceipt(s.ctx, notaryNode.HTTPClient, group.GenesisTransaction, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get privacy group genesis receipt: %w", err)
	}
	if !groupReceipt.Success {
		return fmt.Errorf("privacy group creation failed: %s", groupReceipt.FailureMessage)
	}
	penteAddress := groupReceipt.ContractAddress
	log.Infof("Privacy group contract address: %s", penteAddress)

	// --- Step 2: Deploy NotoTrackerERC20 inside the privacy group ---
	log.Info("Loading NotoTrackerERC20 contract...")
	notoTracker, err := contracts.LoadNotoTrackerERC20Contract()
	if err != nil {
		return err
	}

	var trackerConstructor *abi.Entry
	for _, entry := range notoTracker.ABI {
		if entry.Type == abi.Constructor {
			trackerConstructor = entry
			break
		}
	}

	log.Info("Deploying NotoTrackerERC20 to privacy group...")
	trackerDeployTxID, err := notaryNode.HTTPClient.PrivacyGroups().SendTransaction(s.ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  group.ID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			Bytecode: notoTracker.Bytecode,
			Function: trackerConstructor,
			Input:    pldtypes.RawJSON(`{"name":"NOTO","symbol":"NOTO"}`),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to deploy NotoTrackerERC20: %w", err)
	}

	log.Info("Waiting for NotoTrackerERC20 deployment receipt...")
	trackerReceipt, err := util.WaitForTransactionReceiptFull(s.ctx, notaryNode.HTTPClient, trackerDeployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get NotoTrackerERC20 deployment receipt: %w", err)
	}
	if !trackerReceipt.Success {
		return fmt.Errorf("NotoTrackerERC20 deployment failed: %s", trackerReceipt.FailureMessage)
	}

	var trackerPrivateAddress *pldtypes.EthAddress
	if trackerReceipt.DomainReceipt != nil {
		var domainReceipt pldapi.PenteDomainReceipt
		if err := json.Unmarshal(trackerReceipt.DomainReceipt, &domainReceipt); err == nil {
			if domainReceipt.Receipt != nil && domainReceipt.Receipt.ContractAddress != nil {
				trackerPrivateAddress = domainReceipt.Receipt.ContractAddress
			}
		}
	}
	if trackerPrivateAddress == nil {
		return fmt.Errorf("NotoTrackerERC20 private contract address not found in domain receipt")
	}
	log.Infof("NotoTrackerERC20 deployed at private address: %s", *trackerPrivateAddress)

	// --- Step 3: Deploy Noto domain instance with hooks ---
	log.Info("Deploying Noto with NotoTrackerERC20 hooks...")
	constructorParams := map[string]any{
		"notary":     s.notary,
		"notaryMode": string(nototypes.NotaryModeHooks),
		"options": map[string]any{
			"hooks": map[string]any{
				"publicAddress":  penteAddress.String(),
				"privateAddress": trackerPrivateAddress.String(),
				"privateGroup": map[string]any{
					"salt":    group.ID.String(),
					"members": s.members,
				},
			},
		},
	}

	notoDeployResult := notaryNode.HTTPClient.ForABI(s.ctx, contracts.NotoConstructorABI).
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

	// --- Step 4: Mint initial supply to each node's member identity ---
	for _, memberIdentity := range s.members {
		log.Infof("Minting %d tokens to %s...", s.initialMintAmount, memberIdentity)
		mintParams := &nototypes.MintParams{
			To:     memberIdentity,
			Amount: pldtypes.Uint64ToUint256(uint64(s.initialMintAmount)),
		}
		mintJSON, err := json.Marshal(mintParams)
		if err != nil {
			return fmt.Errorf("failed to marshal mint params for %s: %w", memberIdentity, err)
		}
		mintTxID, err := notaryNode.HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
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
			return fmt.Errorf("failed to send mint transaction for %s: %w", memberIdentity, err)
		}
		mintReceipt, err := util.WaitForTransactionReceipt(s.ctx, notaryNode.HTTPClient, *mintTxID, 60*time.Second)
		if err != nil {
			return fmt.Errorf("failed to get mint receipt for %s: %w", memberIdentity, err)
		}
		if !mintReceipt.Success {
			return fmt.Errorf("mint for %s failed: %s", memberIdentity, mintReceipt.FailureMessage)
		}
		log.Infof("Minted %d tokens to %s", s.initialMintAmount, memberIdentity)
	}

	// Create a receipt listener on the notary node.
	var latestSequence *uint64
	qb := query.NewQueryBuilder().Equal("domain", "noto").Sort("-sequence").Limit(1)
	receipts, err := notaryNode.HTTPClient.PTX().QueryTransactionReceipts(s.ctx, qb.Query())
	if err == nil && len(receipts) > 0 {
		seq := receipts[0].Sequence
		latestSequence = &seq
		log.Infof("Found latest sequence: %d, will start listener from sequence above this", seq)
	} else {
		log.Info("No existing Noto receipts found, starting listener from beginning")
	}

	_, _ = notaryNode.HTTPClient.PTX().DeleteReceiptListener(s.ctx, s.listenerName)

	txType := pldapi.TransactionTypePrivate.Enum()
	_, err = notaryNode.HTTPClient.PTX().CreateReceiptListener(s.ctx, &pldapi.TransactionReceiptListener{
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

	log.Info("Noto pente tracker test setup complete")
	return nil
}

func (s *notoPenteTrackerSuite) Subscribe() (rpcclient.Subscription, error) {
	nodes := s.runner.GetNodes()
	sub, err := nodes[0].WSClient.PTX().SubscribeReceipts(s.ctx, s.listenerName)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to noto receipts: %w", err)
	}
	s.sub = sub
	return sub, nil
}

// OnReceiptBatch is called by the runner every N completions. It queries the receipts
// for the batch and records any failures. Concurrent calls are safe via resultsMu.
func (s *notoPenteTrackerSuite) OnReceiptBatch(ids []string) {
	nodes := s.runner.GetNodes()
	client := nodes[0].HTTPClient

	queryIDs := make([]any, len(ids))
	for i, id := range ids {
		queryIDs[i] = id
	}
	qb := query.NewQueryBuilder().In("id", queryIDs).Limit(len(ids))
	pageReceipts, err := client.PTX().QueryTransactionReceipts(s.ctx, qb.Query())

	var batchFailures []string
	if err != nil {
		batchFailures = append(batchFailures, fmt.Sprintf("failed to query receipts for batch of %d: %v", len(ids), err))
	} else {
		if len(pageReceipts) != len(ids) {
			batchFailures = append(batchFailures, fmt.Sprintf("queried %d IDs but got %d receipts", len(ids), len(pageReceipts)))
		}
		for _, receipt := range pageReceipts {
			if !receipt.Success {
				batchFailures = append(batchFailures, fmt.Sprintf("tx %s expected success but failed: %s", receipt.ID, receipt.FailureMessage))
			}
		}
	}

	log.Infof("notoPenteTrackerSuite: rolling check batch=%d failures=%d", len(ids), len(batchFailures))

	if len(batchFailures) > 0 {
		for _, f := range batchFailures {
			log.Errorf("notoPenteTrackerSuite: rolling check failure: %s", f)
		}
		s.resultsMu.Lock()
		s.failures = append(s.failures, batchFailures...)
		s.resultsMu.Unlock()
	}
}

func (s *notoPenteTrackerSuite) Unsubscribe() {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(s.ctx); err != nil {
			log.Errorf("Error unsubscribing from subscription: %s", err.Error())
		} else {
			log.Info("Successfully unsubscribed")
		}
		s.sub = nil
	}
}

func (s *notoPenteTrackerSuite) Cleanup() {
	nodes := s.runner.GetNodes()
	if len(nodes) > 0 {
		_, err := nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, s.listenerName)
		if err != nil {
			log.Debugf("Failed to delete receipt listener %s: %v", s.listenerName, err)
		} else {
			log.Infof("Successfully deleted receipt listener: %s", s.listenerName)
		}
	}
}

func (s *notoPenteTrackerSuite) NewWorker(startTime int64, workerID int) TestCase {
	return &notoPenteTrackerWorker{
		testBase: testBase{
			ctx:       s.ctx,
			startTime: startTime,
			workerID:  workerID,
		},
		suite:               s,
		notoContractAddress: s.notoContractAddress,
		members:             s.members,
		runner:              s.runner,
	}
}

func (s *notoPenteTrackerSuite) PostRun() error {
	s.resultsMu.Lock()
	defer s.resultsMu.Unlock()

	log.Infof("notoPenteTrackerSuite: post-run complete, total failures=%d", len(s.failures))

	if len(s.failures) > 0 {
		return fmt.Errorf("rolling check failures (%d): %s", len(s.failures), strings.Join(s.failures, "; "))
	}
	return nil
}

type notoPenteTrackerWorker struct {
	testBase
	suite               *notoPenteTrackerSuite
	notoContractAddress *pldtypes.EthAddress
	members             []string
	runner              Runner
}

func (tc *notoPenteTrackerWorker) Name() conf.TestName {
	return conf.PerfTestNotoPenteTracker
}

func (tc *notoPenteTrackerWorker) RunOnce(iterationCount int) (string, error) {
	nodes := tc.runner.GetNodes()
	senderIdx := tc.workerID % len(nodes)
	recipientIdx := (iterationCount + 1) % len(nodes)
	recipient := tc.members[recipientIdx]

	transferParams := &nototypes.TransferParams{
		To:     recipient,
		Amount: pldtypes.Uint64ToUint256(1),
	}
	transferJSON, err := json.Marshal(transferParams)
	if err != nil {
		return "", fmt.Errorf("failed to marshal transfer params: %w", err)
	}

	txID, err := nodes[senderIdx].HTTPClient.PTX().SendTransaction(tc.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "noto",
			Function:       "transfer",
			To:             tc.notoContractAddress,
			From:           "member",
			Data:           pldtypes.RawJSON(transferJSON),
			IdempotencyKey: util.GetIdempotencyKey(tc.startTime, tc.workerID, iterationCount),
		},
		ABI: nototypes.NotoABI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send noto transfer transaction: %w", err)
	}

	return txID.String(), nil
}
