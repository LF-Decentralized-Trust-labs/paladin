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

	nototypes "github.com/LFDT-Paladin/paladin/test/internal/contracts"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/test/internal/conf"
	"github.com/LFDT-Paladin/paladin/test/internal/util"
	"github.com/google/uuid"
	"github.com/hyperledger-firefly/signer/pkg/abi"
	log "github.com/sirupsen/logrus"
)

const notoTransferDefaultListenerName = "nototransferlistener"
const notoTransferDefaultInitialMintAmount = int64(1000000)
const notoTransferDefaultNotoCount = 1

var notoTransferConstructorABI = abi.ABI{
	{Type: abi.Constructor, Inputs: abi.ParameterArray{
		{Name: "notary", Type: "string"},
		{Name: "notaryMode", Type: "string"},
	}},
}

// notoInstance holds the per-Noto-contract state established during Setup.
type notoInstance struct {
	contractAddress *pldtypes.EthAddress
	notary          string
	sender          string
	// signingKey is the short wallet key used in From fields (no @node suffix).
	signingKey string
}

type notoTransferSuite struct {
	ctx               context.Context
	runner            Runner
	notos             []notoInstance // one entry per deployed Noto instance
	notoCount         int
	recipients        []string // one per configured node: "recipient@<nodeName>"
	listenerName      string
	initialMintAmount int64
	sub               rpcclient.Subscription

	resultsMu sync.Mutex
	failures  []string
}

type notoTransferOptions struct {
	ListenerName      *string `json:"listenerName"`
	InitialMintAmount *int64  `json:"initialMintAmount"`
	NotoCount         *int    `json:"notoCount"`
}

func NewNotoTransferSuite(ctx context.Context, runner Runner) *notoTransferSuite {
	return &notoTransferSuite{ctx: ctx, runner: runner}
}

func (s *notoTransferSuite) parseOptions(options map[string]any) error {
	var input notoTransferOptions
	if options != nil {
		raw, err := json.Marshal(options)
		if err != nil {
			return fmt.Errorf("invalid test.options for noto_transfer: %w", err)
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return fmt.Errorf("unable to parse test.options for noto_transfer: %w", err)
		}
	}
	s.listenerName = confutil.StringNotEmpty(input.ListenerName, notoTransferDefaultListenerName)
	s.initialMintAmount = confutil.Int64Min(input.InitialMintAmount, 1, notoTransferDefaultInitialMintAmount)
	s.notoCount = confutil.IntMin(input.NotoCount, 1, notoTransferDefaultNotoCount)
	return nil
}

func (s *notoTransferSuite) Setup() error {
	nodes := s.runner.GetNodes()
	if len(nodes) < 1 {
		return fmt.Errorf("noto_transfer requires at least 1 node")
	}

	testCfg := s.runner.GetTestConfig()
	if err := s.parseOptions(testCfg.Options); err != nil {
		return err
	}

	notaryNode := nodes[0]
	s.recipients = make([]string, len(nodes))
	for i, n := range nodes {
		s.recipients[i] = fmt.Sprintf("recipient@%s", n.Config.Name)
	}

	// Deploy each Noto instance with its own notary and sender identity.
	for i := 0; i < s.notoCount; i++ {
		notaryIdentity := fmt.Sprintf("notary%d@%s", i, notaryNode.Config.Name)
		senderIdentity := fmt.Sprintf("sender%d@%s", i, notaryNode.Config.Name)
		notaryKey := fmt.Sprintf("notary%d", i)

		log.Infof("Deploying Noto instance %d: notary=%s sender=%s", i, notaryIdentity, senderIdentity)

		constructorParams := map[string]any{
			"notary":     notaryIdentity,
			"notaryMode": string(nototypes.NotaryModeBasic),
		}
		deployResult := notaryNode.HTTPClient.ForABI(s.ctx, notoTransferConstructorABI).
			Private().
			Domain("noto").
			Constructor().
			From(notaryKey).
			Inputs(constructorParams).
			Send().
			Wait(60 * time.Second)
		if deployResult.Error() != nil {
			return fmt.Errorf("failed to deploy Noto instance %d: %w", i, deployResult.Error())
		}
		if deployResult.Receipt().ContractAddress == nil {
			return fmt.Errorf("Noto instance %d contract address not found in deployment receipt", i)
		}
		contractAddress := deployResult.Receipt().ContractAddress
		log.Infof("Noto instance %d deployed at address: %s", i, *contractAddress)

		// Mint initial supply to this instance's sender.
		log.Infof("Minting initial supply of %d to %s (Noto instance %d)...", s.initialMintAmount, senderIdentity, i)
		mintParams := &nototypes.MintParams{
			To:     senderIdentity,
			Amount: pldtypes.Uint64ToUint256(uint64(s.initialMintAmount)),
		}
		mintJSON, err := json.Marshal(mintParams)
		if err != nil {
			return fmt.Errorf("failed to marshal mint params for instance %d: %w", i, err)
		}
		mintTxID, err := notaryNode.HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type:     pldapi.TransactionTypePrivate.Enum(),
				Domain:   "noto",
				Function: "mint",
				To:       contractAddress,
				From:     notaryKey,
				Data:     pldtypes.RawJSON(mintJSON),
			},
			ABI: nototypes.NotoABI,
		})
		if err != nil {
			return fmt.Errorf("failed to send mint transaction for instance %d: %w", i, err)
		}
		mintReceipt, err := util.WaitForTransactionReceipt(s.ctx, notaryNode.HTTPClient, *mintTxID, 60*time.Second)
		if err != nil {
			return fmt.Errorf("failed to get mint receipt for instance %d: %w", i, err)
		}
		if !mintReceipt.Success {
			return fmt.Errorf("mint for instance %d failed: %s", i, mintReceipt.FailureMessage)
		}
		log.Infof("Noto instance %d: minted %d tokens to %s", i, s.initialMintAmount, senderIdentity)

		s.notos = append(s.notos, notoInstance{
			contractAddress: contractAddress,
			notary:          notaryIdentity,
			sender:          senderIdentity,
			signingKey:      fmt.Sprintf("sender%d", i),
		})
	}

	log.Infof("Deployed %d Noto instance(s), recipients: %s", s.notoCount, strings.Join(s.recipients, ", "))

	// Create a single receipt listener on the notary node covering all Noto instances.
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

	log.Info("Noto transfer test setup complete")
	return nil
}

func (s *notoTransferSuite) Subscribe() (rpcclient.Subscription, error) {
	nodes := s.runner.GetNodes()
	sub, err := nodes[0].WSClient.PTX().SubscribeReceipts(s.ctx, s.listenerName)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to noto receipts: %w", err)
	}
	s.sub = sub
	return sub, nil
}

// OnReceiptBatch is called by the runner every N completions. It validates each txID
// and records any failures. Concurrent calls are safe via resultsMu.
func (s *notoTransferSuite) OnReceiptBatch(ids []string) {
	nodes := s.runner.GetNodes()
	client := nodes[0].HTTPClient

	var batchFailures []string
	for _, txID := range ids {
		parsedID, err := uuid.Parse(txID)
		if err != nil {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s has invalid UUID: %v", txID, err))
			continue
		}
		receipt, err := client.PTX().GetTransactionReceiptFull(s.ctx, parsedID)
		if err != nil || receipt == nil {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s failed to fetch receipt: %v", txID, err))
			continue
		}
		if !receipt.Success {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s expected success but failed: %s", txID, receipt.FailureMessage))
		}
	}

	log.Infof("notoTransferSuite: rolling check batch=%d failures=%d", len(ids), len(batchFailures))

	if len(batchFailures) > 0 {
		for _, f := range batchFailures {
			log.Errorf("notoTransferSuite: rolling check failure: %s", f)
		}
		s.resultsMu.Lock()
		s.failures = append(s.failures, batchFailures...)
		s.resultsMu.Unlock()
	}
}

func (s *notoTransferSuite) Unsubscribe() {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(s.ctx); err != nil {
			log.Errorf("Error unsubscribing from subscription: %s", err.Error())
		} else {
			log.Info("Successfully unsubscribed")
		}
		s.sub = nil
	}
}

func (s *notoTransferSuite) Cleanup() {
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

func (s *notoTransferSuite) NewWorker(startTime int64, workerID int) TestCase {
	return &notoTransferWorker{
		testBase: testBase{
			ctx:       s.ctx,
			startTime: startTime,
			workerID:  workerID,
		},
		suite:      s,
		notos:      s.notos,
		recipients: s.recipients,
		runner:     s.runner,
	}
}

func (s *notoTransferSuite) PostRun() error {
	s.resultsMu.Lock()
	defer s.resultsMu.Unlock()

	log.Infof("notoTransferSuite: post-run complete, total failures=%d", len(s.failures))

	if len(s.failures) > 0 {
		return fmt.Errorf("rolling check failures (%d): %s", len(s.failures), strings.Join(s.failures, "; "))
	}
	return nil
}

type notoTransferWorker struct {
	testBase
	suite      *notoTransferSuite
	notos      []notoInstance
	recipients []string
	runner     Runner
}

func (tc *notoTransferWorker) Name() conf.TestName {
	return conf.PerfTestNotoTransfer
}

func (tc *notoTransferWorker) RunOnce(iterationCount int) (string, error) {
	nodes := tc.runner.GetNodes()
	noto := tc.notos[iterationCount%len(tc.notos)]
	target := tc.recipients[iterationCount%len(nodes)]

	transferParams := &nototypes.TransferParams{
		To:     target,
		Amount: pldtypes.Uint64ToUint256(1),
	}
	transferJSON, err := json.Marshal(transferParams)
	if err != nil {
		return "", fmt.Errorf("failed to marshal transfer params: %w", err)
	}

	txID, err := nodes[0].HTTPClient.PTX().SendTransaction(tc.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "noto",
			Function:       "transfer",
			To:             noto.contractAddress,
			From:           noto.signingKey,
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
