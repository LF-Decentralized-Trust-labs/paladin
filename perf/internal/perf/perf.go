// Copyright © 2025 Kaleido, Inc.
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

package perf

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/perf/internal/util"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	dto "github.com/prometheus/client_model/go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

//go:embed abis/SimpleStorage.json
var simpleStorageBuildJSON []byte

type simpleStorageContractData struct {
	ABI      abi.ABI           `json:"abi"`
	Bytecode pldtypes.HexBytes `json:"bytecode"`
}

func loadSimpleStorageContract() (*simpleStorageContractData, error) {
	var contractData simpleStorageContractData
	if err := json.Unmarshal(simpleStorageBuildJSON, &contractData); err != nil {
		return nil, fmt.Errorf("failed to parse embedded SimpleStorage.json: %w", err)
	}
	return &contractData, nil
}

const workerPrefix = "worker-"

var METRICS_NAMESPACE = "pldperf"
var METRICS_SUBSYSTEM = "runner"

var totalActionsCounter = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: METRICS_NAMESPACE,
	Name:      "actions_submitted_total",
	Subsystem: METRICS_SUBSYSTEM,
})

var receivedEventsCounter = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: METRICS_NAMESPACE,
	Name:      "received_events_total",
	Subsystem: METRICS_SUBSYSTEM,
})

var incompleteEventsCounter = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: METRICS_NAMESPACE,
	Name:      "incomplete_events_total",
	Subsystem: METRICS_SUBSYSTEM,
})

var delinquentMsgsCounter = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: METRICS_NAMESPACE,
	Name:      "deliquent_msgs_total",
	Subsystem: METRICS_SUBSYSTEM,
})

var perfTestDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: METRICS_NAMESPACE,
	Subsystem: METRICS_SUBSYSTEM,
	Name:      "perf_test_duration_seconds",
	Buckets:   []float64{1.0, 2.0, 5.0, 10.0, 30.0},
}, []string{"test"})

func Init() {
	prometheus.Register(delinquentMsgsCounter)
	prometheus.Register(receivedEventsCounter)
	prometheus.Register(incompleteEventsCounter)
	prometheus.Register(totalActionsCounter)
	prometheus.Register(perfTestDurationHistogram)
}

func getMetricVal(collector prometheus.Collector) float64 {
	collectorChannel := make(chan prometheus.Metric, 1)
	collector.Collect(collectorChannel)
	metric := dto.Metric{}
	err := (<-collectorChannel).Write(&metric)
	if err != nil {
		log.Errorf("error writing metric: %s", err)
	}
	if metric.Counter != nil {
		return *metric.Counter.Value
	} else if metric.Gauge != nil {
		return *metric.Gauge.Value
	}
	return 0
}

type PerfRunner interface {
	Init() error
	Start() error
}

type TestCase interface {
	WorkerID() int
	RunOnce(iterationCount int) (trackingID string, err error)
	Name() conf.TestName
	ActionsPerLoop() int
}

type inflightTest struct {
	time     time.Time
	testCase TestCase
}

type summary struct {
	mutex        *sync.Mutex
	rampSummary  int64
	totalSummary int64
}
type perfRunner struct {
	bfr               chan int
	cfg               *conf.RunnerConfig
	httpClients       []pldclient.PaladinClient // HTTP clients for all nodes
	wsClient          pldclient.PaladinWSClient
	ctx               context.Context
	shutdown          context.CancelFunc
	stopping          bool
	closingWebsocket  bool // Flag to indicate websocket is being closed in cleanup
	nodeKilled        bool // Flag to stop accepting new transactions after node kill
	waitingForRestart bool // Flag to indicate we're waiting for node restart

	startTime     int64
	endSendTime   int64
	endTime       int64
	startRampTime int64
	endRampTime   int64

	totalWorkers  int
	reportBuilder *util.Report
	sendTime      *util.Latency
	receiveTime   *util.Latency
	totalTime     *util.Latency
	summary       summary
	msgTimeMap    sync.Map
	workerIDMap   sync.Map

	wsReceivers   map[string]chan bool
	subscriptions []rpcclient.Subscription

	// Listener name for cleanup
	listenerName string

	// Pente test specific fields
	privacyGroupID  *pldtypes.HexBytes
	contractAddress *pldtypes.EthAddress
	nodeManager     NodeManager

	// Node kill coordination channels (one set per worker)
	pauseRequests []chan struct{} // Channels to signal each worker to pause
	pauseAcks     []chan struct{} // Channels for each worker to acknowledge they've paused
	resumeSignals []chan struct{} // Channels to signal each worker to resume
}

func New(config *conf.RunnerConfig, reportBuilder *util.Report) PerfRunner {
	if config.LogLevel != "" {
		if level, err := log.ParseLevel(config.LogLevel); err == nil {
			log.SetLevel(level)
		}
	}

	totalWorkers := config.Test.Workers

	// Create channel based dispatch for workers
	wsReceivers := make(map[string]chan bool)
	for i := 0; i < totalWorkers; i++ {
		prefixedWorkerID := fmt.Sprintf("%s%d", workerPrefix, i)
		wsReceivers[prefixedWorkerID] = make(chan bool)
	}

	ctx, cancel := context.WithCancel(context.Background())

	startRampTime := time.Now().Unix()
	endRampTime := time.Now().Unix() + int64(config.RampLength.Seconds())
	startTime := endRampTime
	endTime := startTime + int64(config.Length.Seconds())

	pr := &perfRunner{
		bfr:           make(chan int, totalWorkers),
		cfg:           config,
		ctx:           ctx,
		shutdown:      cancel,
		startRampTime: startRampTime,
		endRampTime:   endRampTime,
		startTime:     startTime,
		endTime:       endTime,
		reportBuilder: reportBuilder,
		sendTime:      &util.Latency{},
		receiveTime:   &util.Latency{},
		totalTime:     &util.Latency{},
		msgTimeMap:    sync.Map{},
		workerIDMap:   sync.Map{},
		summary: summary{
			totalSummary: 0,
			mutex:        &sync.Mutex{},
		},
		wsReceivers:   wsReceivers,
		totalWorkers:  totalWorkers,
		pauseRequests: make([]chan struct{}, totalWorkers),
		pauseAcks:     make([]chan struct{}, totalWorkers),
		resumeSignals: make([]chan struct{}, totalWorkers),
	}
	// Initialize channels for each worker
	for i := 0; i < totalWorkers; i++ {
		pr.pauseRequests[i] = make(chan struct{})
		pr.pauseAcks[i] = make(chan struct{})
		pr.resumeSignals[i] = make(chan struct{})
	}
	return pr
}

func (pr *perfRunner) Init() (err error) {
	// All tests require at least one node
	if len(pr.cfg.Nodes) == 0 {
		return fmt.Errorf("at least one node must be configured")
	}

	// Create HTTP clients for all configured nodes
	pr.httpClients = make([]pldclient.PaladinClient, len(pr.cfg.Nodes))
	for i, node := range pr.cfg.Nodes {
		httpConfig := pr.cfg.HTTPConfig
		httpConfig.URL = node.HTTPEndpoint

		pr.httpClients[i], err = pldclient.New().HTTP(pr.ctx, &httpConfig)
		if err != nil {
			return fmt.Errorf("failed to create HTTP client for node %d: %w", i, err)
		}
	}

	// Use first node's WSEndpoint for WebSocket connection
	wsConfig := pr.cfg.WSConfig
	wsConfig.URL = pr.cfg.Nodes[0].WSEndpoint
	pr.wsClient, err = pr.httpClients[0].WebSocket(pr.ctx, &wsConfig)
	if err != nil {
		return err
	}

	// Initialize node manager if node kill config is provided
	if pr.cfg.NodeKillConfig != nil {
		pr.nodeManager = NewNodeManager(pr.cfg.NodeKillConfig, pr.cfg.Nodes)
	}

	return nil
}

func (pr *perfRunner) setupPenteTest() error {
	// Load contract bytecode and ABI from embedded SimpleStorage.json
	simpleStorage, err := loadSimpleStorageContract()
	if err != nil {
		return err
	}

	// Build members list from node names: "member@" + node name
	members := make([]string, len(pr.cfg.Nodes))
	for i, node := range pr.cfg.Nodes {
		members[i] = fmt.Sprintf("member@%s", node.Name)
	}

	// Create privacy group using pgroup_createGroup API
	log.Info("Creating privacy group for pente test...")
	group, err := pr.httpClients[0].PrivacyGroups().CreateGroup(pr.ctx, &pldapi.PrivacyGroupInput{
		Domain:  "pente",
		Members: members,
		Name:    "perf-test-privacy-group",
		Configuration: map[string]string{
			"evmVersion":           "shanghai",
			"externalCallsEnabled": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create privacy group: %w", err)
	}

	pr.privacyGroupID = &group.ID
	log.Infof("Privacy group created with ID: %s", group.ID)

	// Wait for privacy group creation receipt by polling GetTransactionReceipt
	log.Info("Waiting for privacy group creation receipt...")
	receipt, err := pr.waitForTransactionReceipt(group.GenesisTransaction, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get privacy group creation receipt: %w", err)
	}
	if !receipt.Success {
		return fmt.Errorf("privacy group creation transaction failed")
	}
	log.Info("Privacy group creation confirmed")

	var function *abi.Entry
	for _, entry := range simpleStorage.ABI {
		if entry.Type == abi.Constructor {
			function = entry
			break
		}
	}

	// Deploy contract to privacy group (omit `to`, provide `bytecode` and constructor function)
	log.Info("Deploying contract to privacy group...")
	deployTxID, err := pr.httpClients[0].PrivacyGroups().SendTransaction(pr.ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  *pr.privacyGroupID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			Bytecode: simpleStorage.Bytecode,
			Function: function,
			Input:    pldtypes.RawJSON(fmt.Sprintf("[%d]", 0)),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to deploy contract: %w", err)
	}

	// Wait for contract deployment receipt - use GetTransactionReceiptFull to get domain receipt
	log.Info("Waiting for contract deployment receipt...")
	deployReceipt, err := pr.waitForTransactionReceiptFull(deployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get contract deployment receipt: %w", err)
	}
	if !deployReceipt.Success {
		return fmt.Errorf("contract deployment transaction failed")
	}

	// Extract contract address from domain receipt
	if deployReceipt.DomainReceipt != nil {
		var domainReceipt map[string]interface{}
		if err := json.Unmarshal(deployReceipt.DomainReceipt, &domainReceipt); err == nil {
			if receiptData, ok := domainReceipt["receipt"].(map[string]interface{}); ok {
				if addrStr, ok := receiptData["contractAddress"].(string); ok {
					addr := pldtypes.MustEthAddress(addrStr)
					pr.contractAddress = addr
					log.Infof("Contract deployed at address: %s", *addr)
				}
			}
		}
	}

	if pr.contractAddress == nil {
		return fmt.Errorf("contract address not found in deployment receipt")
	}

	return nil
}

func (pr *perfRunner) waitForTransactionReceipt(txID uuid.UUID, timeout time.Duration) (*pldapi.TransactionReceipt, error) {
	deadline := time.Now().Add(timeout)
	checkInterval := 1 * time.Second

	for time.Now().Before(deadline) {
		receipt, err := pr.httpClients[0].PTX().GetTransactionReceipt(pr.ctx, txID)
		if err == nil && receipt != nil {
			return receipt, nil
		}

		select {
		case <-pr.ctx.Done():
			return nil, pr.ctx.Err()
		case <-time.After(checkInterval):
			// Continue polling
		}
	}

	return nil, fmt.Errorf("timeout waiting for transaction receipt: %s", txID)
}

func (pr *perfRunner) waitForTransactionReceiptFull(txID uuid.UUID, timeout time.Duration) (*pldapi.TransactionReceiptFull, error) {
	deadline := time.Now().Add(timeout)
	checkInterval := 1 * time.Second

	for time.Now().Before(deadline) {
		receipt, err := pr.httpClients[0].PTX().GetTransactionReceiptFull(pr.ctx, txID)
		if err == nil && receipt != nil {
			return receipt, nil
		}

		select {
		case <-pr.ctx.Done():
			return nil, pr.ctx.Err()
		case <-time.After(checkInterval):
			// Continue polling
		}
	}

	return nil, fmt.Errorf("timeout waiting for transaction receipt: %s", txID)
}

func (pr *perfRunner) subscribeToPenteReceiptListener() error {
	// Create receipt listener for pente transactions, starting from latest sequence
	pr.listenerName = "penteperflistener"

	var latestSequence *uint64
	qb := query.NewQueryBuilder().Equal("type", "private").Equal("domain", "pente").Sort("-sequence").Limit(1)
	receipts, err := pr.httpClients[0].PTX().QueryTransactionReceipts(pr.ctx, qb.Query())
	if err == nil && len(receipts) > 0 {
		seq := receipts[0].Sequence
		latestSequence = &seq
		log.Infof("Found latest sequence: %d, will start listener from sequence above this", seq)
	} else {
		log.Info("No existing receipts found, starting listener from beginning")
	}

	_, err = pr.httpClients[0].PTX().DeleteReceiptListener(pr.ctx, pr.listenerName)
	if err != nil {
		log.Debugf("No existing listener to delete (or delete failed): %v", err)
	}

	txType := pldapi.TransactionTypePrivate.Enum()
	_, err = pr.httpClients[0].PTX().CreateReceiptListener(pr.ctx, &pldapi.TransactionReceiptListener{
		Name: pr.listenerName,
		Filters: pldapi.TransactionReceiptFilters{
			Type:          &txType,
			Domain:        "pente",
			SequenceAbove: latestSequence,
		},
		Options: pldapi.TransactionReceiptListenerOptions{
			DomainReceipts: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create receipt listener: %w", err)
	}

	// Subscribe to receipts using the same pattern as public contract test
	sub, err := pr.wsClient.PTX().SubscribeReceipts(pr.ctx, pr.listenerName)
	if err != nil {
		return fmt.Errorf("failed to subscribe to pente receipts: %w", err)
	}

	pr.subscriptions = append(pr.subscriptions, sub)
	go pr.batchEventLoop(sub)

	return nil
}

func (pr *perfRunner) setupPublicContractTest() error {
	// Load contract bytecode and ABI from embedded SimpleStorage.json
	simpleStorage, err := loadSimpleStorageContract()
	if err != nil {
		return err
	}

	// Deploy contract as public transaction (omit `to`, provide `bytecode` and `abi`)
	log.Info("Deploying contract as public transaction...")
	deployTxID, err := pr.httpClients[0].PTX().SendTransaction(pr.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "deploy",
			Data: pldtypes.RawJSON(fmt.Sprintf("[%d]", 0)),
		},
		Bytecode: simpleStorage.Bytecode,
		ABI:      simpleStorage.ABI,
	})
	if err != nil {
		return fmt.Errorf("failed to deploy contract: %w", err)
	}

	// Wait for contract deployment receipt
	log.Info("Waiting for contract deployment receipt...")
	deployReceipt, err := pr.waitForTransactionReceiptFull(*deployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get contract deployment receipt: %w", err)
	}
	if !deployReceipt.Success {
		return fmt.Errorf("contract deployment transaction failed")
	}

	// Extract contract address from receipt data (public transactions have it directly in the receipt)
	if deployReceipt.ContractAddress != nil {
		pr.contractAddress = deployReceipt.ContractAddress
		log.Infof("Contract deployed at address: %s", *deployReceipt.ContractAddress)
	}

	if pr.contractAddress == nil {
		return fmt.Errorf("contract address not found in deployment receipt")
	}

	return nil
}

func (pr *perfRunner) Start() (err error) {
	log.Infof("Running test:\n%+v", pr.cfg)

	test := pr.cfg.Test

	// Setup listeners based on test type
	switch test.Name {
	case conf.PerfTestPublicContract:
		log.Infof("Running public contract test using first configured node: %s", pr.cfg.Nodes[0].HTTPEndpoint)
		if err = pr.setupPublicContractTest(); err != nil {
			return fmt.Errorf("failed to setup public contract test: %w", err)
		}
		if err = pr.subscribeToPublicContractListener(); err != nil {
			return err
		}
	case conf.PerfTestPrivateTransactionNodeRestart:
		log.Infof("Running private transaction node restart test")
		if err = pr.setupPenteTest(); err != nil {
			return fmt.Errorf("failed to setup pente test: %w", err)
		}
		if err = pr.subscribeToPenteReceiptListener(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown test case '%s'", test.Name)
	}

	// Start workers
	log.Infof("Starting %d workers for case \"%s\"", test.Workers, test.Name)
	id := 0
	for iWorker := 0; iWorker < test.Workers; iWorker++ {
		var tc TestCase

		switch test.Name {
		case conf.PerfTestPublicContract:
			tc = newPublicContractTestWorker(pr, id, test.ActionsPerLoop)
		case conf.PerfTestPrivateTransactionNodeRestart:
			if pr.privacyGroupID == nil || pr.contractAddress == nil {
				return fmt.Errorf("pente test not properly initialized")
			}
			tc = newPrivateTransactionNodeRestartTestWorker(pr, id, test.ActionsPerLoop, pr.privacyGroupID, pr.contractAddress, pr.httpClients)
		default:
			return fmt.Errorf("unknown test case '%s'", test.Name)
		}

		delayPerWorker := pr.cfg.RampLength / time.Duration(test.Workers)

		go func(i int) {
			// Delay the start of the next worker by (ramp time) / (number of workers)
			if delayPerWorker > 0 {
				time.Sleep(delayPerWorker * time.Duration(i))
				log.Infof("Ramping up. Starting next worker after waiting %v", delayPerWorker)
			}
			err := pr.runLoop(tc)
			if err != nil {
				log.Errorf("Worker %d failed: %s", tc.WorkerID(), err)
			}
		}(iWorker)
		id++
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	signal.Notify(signalCh, os.Kill)
	signal.Notify(signalCh, syscall.SIGTERM)
	signal.Notify(signalCh, syscall.SIGQUIT)
	signal.Notify(signalCh, syscall.SIGKILL)

	i := 0
	lastCheckedTime := time.Now()

	rateLimiter := rate.NewLimiter(rate.Limit(math.MaxFloat64), math.MaxInt)

	if pr.cfg.MaxSubmissionsPerSecond > 0 {
		rateLimiter = rate.NewLimiter(rate.Limit(pr.cfg.MaxSubmissionsPerSecond), pr.cfg.MaxSubmissionsPerSecond)
	}
	log.Infof("Sending rate: %f per second with %d burst", rateLimiter.Limit(), rateLimiter.Burst())

	// Setup node kill timer if configured
	var nodeKillTimer *time.Timer
	var nodeKillTimerCh <-chan time.Time
	if pr.cfg.NodeKillConfig != nil && pr.cfg.NodeKillConfig.KillInterval > 0 {
		nodeKillTimer = time.NewTimer(pr.cfg.NodeKillConfig.KillInterval)
		nodeKillTimerCh = nodeKillTimer.C
		log.Infof("Node kill timer started, will kill a node after %v", pr.cfg.NodeKillConfig.KillInterval)
		defer func() {
			if nodeKillTimer != nil {
				nodeKillTimer.Stop()
			}
		}()
	}
perfLoop:
	for pr.IsDaemon() || time.Now().Unix() < pr.endTime {
		timeout := time.After(60 * time.Second)
		// If we've been given a maximum number of actions to perform, check if we're done
		if pr.cfg.MaxActions > 0 && int64(getMetricVal(totalActionsCounter)) >= pr.cfg.MaxActions {
			break perfLoop
		}

		select {
		case <-signalCh:
			break perfLoop
		case <-nodeKillTimerCh:
			// Clear the timer channel since the timer has fired
			nodeKillTimerCh = nil

			// Don't kill the first node (index 0) as it has the websocket connection
			// If there's only one node, skip killing and restart the timer
			// TODO: handle the websocket needing to reconnect so that any node could be killed
			if len(pr.cfg.Nodes) == 1 {
				log.Debug("Only one node configured, skipping kill to preserve websocket connection")
				// Restart the timer since we skipped the kill
				if nodeKillTimer != nil {
					nodeKillTimer.Stop()
				}
				nodeKillTimer = time.NewTimer(pr.cfg.NodeKillConfig.KillInterval)
				nodeKillTimerCh = nodeKillTimer.C
				continue
			}

			// Randomly select from nodes 1 onwards (excluding first node)
			nodeIndex := 1 + rand.Intn(len(pr.cfg.Nodes)-1)
			log.Infof("Randomly selected node %d (%s) to kill (excluding first node which has websocket connection)", nodeIndex, pr.cfg.Nodes[nodeIndex].Name)

			// Request pause from all workers
			log.Info("Requesting workers to pause for node kill")
			for i := 0; i < pr.totalWorkers; i++ {
				pr.pauseRequests[i] <- struct{}{}
			}

			// Wait for all workers to acknowledge they've paused
			log.Infof("Waiting for %d workers to acknowledge pause", pr.totalWorkers)
			for i := 0; i < pr.totalWorkers; i++ {
				<-pr.pauseAcks[i]
			}
			log.Info("All workers have paused, proceeding with node kill")

			// Kill the selected node
			if err := pr.nodeManager.KillNode(pr.ctx, nodeIndex); err != nil {
				log.Errorf("Failed to kill node %d: %v", nodeIndex, err)
				// Resume workers even on error
				for i := 0; i < pr.totalWorkers; i++ {
					pr.resumeSignals[i] <- struct{}{}
				}
				break perfLoop
			}

			log.Info("Node killed, waiting for restart")

			// Wait for node restart (blocking in this goroutine)
			restartTimeout := pr.cfg.NodeKillConfig.RestartTimeout
			log.Infof("Waiting for node %d to restart (timeout: %v)", nodeIndex, restartTimeout)
			if err := pr.nodeManager.WaitForNodeRestart(pr.ctx, nodeIndex, restartTimeout); err != nil {
				log.Errorf("Node %d did not restart within timeout: %v", nodeIndex, err)
				// Resume workers even on timeout so test can complete
				for i := 0; i < pr.totalWorkers; i++ {
					pr.resumeSignals[i] <- struct{}{}
				}
				break perfLoop
			}

			log.Infof("Node %d has restarted successfully, resuming transaction submissions", nodeIndex)

			// Resume all workers
			for i := 0; i < pr.totalWorkers; i++ {
				pr.resumeSignals[i] <- struct{}{}
			}
			log.Info("All workers have been resumed")

			// Create a new timer after successful restart, so the next kill happens after KillInterval
			if pr.cfg.NodeKillConfig != nil && pr.cfg.NodeKillConfig.KillInterval > 0 {
				if nodeKillTimer != nil {
					nodeKillTimer.Stop()
				}
				nodeKillTimer = time.NewTimer(pr.cfg.NodeKillConfig.KillInterval)
				nodeKillTimerCh = nodeKillTimer.C
				log.Debugf("Node kill timer restarted, next kill will be in %v", pr.cfg.NodeKillConfig.KillInterval)
			}
		case pr.bfr <- i:
			err = rateLimiter.Wait(pr.ctx)
			if err != nil {
				log.Panic(fmt.Errorf("rate limiter failed"))
				break perfLoop
			}
			i++
			if time.Since(lastCheckedTime).Seconds() > pr.cfg.MaxTimePerAction.Seconds() {
				if pr.detectDelinquentMsgs() && pr.cfg.DelinquentAction == conf.DelinquentActionExit {
					break perfLoop
				}
				lastCheckedTime = time.Now()
			}
		case <-timeout:
			if pr.detectDelinquentMsgs() && pr.cfg.DelinquentAction == conf.DelinquentActionExit {
				break perfLoop
			}
			lastCheckedTime = time.Now()
		case <-pr.ctx.Done():
			pr.cleanup()
			break perfLoop
		}

	}

	pr.stopping = true

	// Wait for all pending transactions to complete
	log.Infof("Waiting up to %v for all pending transactions to complete", pr.cfg.CompletionTimeout)
	deadline := time.Now().Add(pr.cfg.CompletionTimeout)
	checkInterval := 2 * time.Second
	idleStart := time.Now()
	lastEventsCount := int64(getMetricVal(receivedEventsCounter))

	for time.Now().Before(deadline) {
		submissionCount := int64(getMetricVal(totalActionsCounter))
		eventsCount := int64(getMetricVal(receivedEventsCounter))

		if eventsCount >= submissionCount {
			log.Infof("All transactions completed! Submitted: %d, Received: %d", submissionCount, eventsCount)
			break
		}

		// Check if we've been idle (no new events) for too long
		if eventsCount > lastEventsCount {
			// Reset idle start time if there are new events
			idleStart = time.Now()
			lastEventsCount = eventsCount
		} else if time.Since(idleStart) > 30*time.Second {
			// If no new events for 30 seconds, log warning but continue waiting until completion timeout
			log.Warnf("No new events received for 30s. Submitted: %d, Received: %d. Continuing to wait...", submissionCount, eventsCount)
			idleStart = time.Now() // Reset to avoid spamming
		}

		log.Infof("Waiting for transactions to complete... Submitted: %d, Received: %d", submissionCount, eventsCount)
		time.Sleep(checkInterval)
	}

	// Final check
	submissionCount := int64(getMetricVal(totalActionsCounter))
	eventsCount := int64(getMetricVal(receivedEventsCounter))
	if eventsCount < submissionCount {
		log.Warnf("Completion timeout reached. Submitted: %d, Received: %d", submissionCount, eventsCount)
	}

	measuredActions := pr.summary.totalSummary
	measuredTime := time.Since(time.Unix(pr.startTime, 0))

	testName := string(pr.cfg.Test.Name)

	tps := util.GenerateTPS(measuredActions, pr.startTime, pr.endSendTime)
	pr.reportBuilder.AddTestRunMetrics(testName, measuredActions, measuredTime, tps, pr.totalTime)
	err = pr.reportBuilder.GenerateHTML()

	if err != nil {
		log.Errorf("failed to generate performance report: %+v", err)
	}

	// we sleep on shutdown / completion to allow for Prometheus metrics to be scraped one final time
	// After 30 seconds workers should be completed, so we check for delinquent messages
	// one last time so metrics are up-to-date
	log.Warn("Runner stopping in 5s")
	time.Sleep(5 * time.Second)
	pr.detectDelinquentMsgs()

	log.Info("Cleaning up")

	pr.cleanup()

	log.Info("Shutdown summary:")
	log.Infof(" - Prometheus metric received_events_total   = %f\n", getMetricVal(receivedEventsCounter))
	log.Infof(" - Prometheus metric incomplete_events_total = %f\n", getMetricVal(incompleteEventsCounter))
	log.Infof(" - Prometheus metric delinquent_msgs_total    = %f\n", getMetricVal(delinquentMsgsCounter))
	log.Infof(" - Prometheus metric actions_submitted_total = %f\n", getMetricVal(totalActionsCounter))
	log.Infof(" - Test duration: %s", measuredTime)
	log.Infof(" - Measured actions: %d", measuredActions)
	log.Infof(" - Measured send TPS: %2f", tps.SendRate)
	log.Infof(" - Measured throughput: %2f", tps.Throughput)
	log.Infof(" - Measured send duration: %s", pr.sendTime)
	log.Infof(" - Measured event receiving duration: %s", pr.receiveTime)
	log.Infof(" - Measured total duration: %s", pr.totalTime)

	return nil
}

func (pr *perfRunner) cleanup() {
	for _, sub := range pr.subscriptions {
		err := sub.Unsubscribe(pr.ctx)
		if err != nil {
			log.Errorf("Error unsubscribing from subscription: %s", err.Error())
		} else {
			log.Info("Successfully unsubscribed")
		}
	}

	// Set flag to indicate websocket is being closed
	pr.closingWebsocket = true
	pr.wsClient.Close()

	// Clean up receipt listener after websocket is closed
	if pr.listenerName != "" {
		_, err := pr.httpClients[0].PTX().DeleteReceiptListener(pr.ctx, pr.listenerName)
		if err != nil {
			log.Debugf("Failed to delete receipt listener %s: %v", pr.listenerName, err)
		} else {
			log.Infof("Successfully deleted receipt listener: %s", pr.listenerName)
		}
	}

}

func (pr *perfRunner) batchEventLoop(sub rpcclient.Subscription) (err error) {
	log.Info("Batch Event loop started")
	for {
		log.Trace("blocking until wsconn.Receive or ctx.Done()")
		select {
		// Wait to receive websocket event
		case subNotification, ok := <-sub.Notifications():
			if !ok {
				// Channel closed - check if it's expected (during cleanup) or unexpected
				if pr.closingWebsocket {
					// Expected: cleanup is closing the websocket
					log.Debug("Websocket channel closed during cleanup")
				} else {
					// Unexpected: channel closed but we're not cleaning up
					log.Errorf("Error receiving websocket: channel closed unexpectedly")
				}
				return
			}
			log.Trace("received from websocket")

			// Handle websocket event
			var batch pldapi.TransactionReceiptBatch
			json.Unmarshal(subNotification.GetResult(), &batch)

			if pr.cfg.LogEvents {
				log.Info("Batch: ", string(subNotification.GetResult()))
			}

			g, _ := errgroup.WithContext(pr.ctx)
			g.SetLimit(-1)

			for _, receipt := range batch.Receipts {
				thisReceipt := receipt
				g.Go(func() error {
					transactionID := thisReceipt.ID.String()
					v, ok := pr.workerIDMap.LoadAndDelete(transactionID)
					if !ok {
						// we cant apply reliable filters to ensure we're only getting back receipts
						// for transactions submitted in this test, so just log and skip any we don't
						// recognuse
						log.Warnf("No worker ID map entry for transaction id: %s", transactionID)
						return nil
					}
					workerID := v.(int)

					if pr.cfg.LogEvents {
						eventJSON, _ := json.Marshal(thisReceipt)
						log.Info("Event: ", string(eventJSON))
					}

					receivedEventsCounter.Inc()
					pr.recordCompletedAction()
					// Release worker so it can continue to its next task
					if !pr.stopping {
						if workerID >= 0 {
							prefixedWorkerID := fmt.Sprintf("%s%d", workerPrefix, workerID)
							// No need for locking as channel have built in support
							pr.wsReceivers[prefixedWorkerID] <- true
						}
					}
					return nil
				})
			}

			// Wait for all go routines to complete
			// The first non-nil go routine will be returned
			// and we will return the error
			log.Debug("Waiting for events from websocket to be handled")
			if err := g.Wait(); err != nil {
				return err
			}
			log.Debug("All events from websocket handled")

			// We have completed all the go routines
			// and can ack the batch
			err := subNotification.Ack(pr.ctx)
			if err != nil {
				log.Errorf("Failed to ack batch receipt: %s", err.Error())
				return err
			}

			pr.summary.mutex.Lock()
			pr.calculateCurrentTps(true)
			pr.summary.mutex.Unlock()
		case <-pr.ctx.Done():
			log.Warnf("Run loop exiting (context cancelled)")
			sub.Unsubscribe(pr.ctx)
			return
		}
	}
}

func (pr *perfRunner) allActionsComplete() bool {
	return pr.cfg.MaxActions > 0 && int64(getMetricVal(totalActionsCounter)) >= pr.cfg.MaxActions
}

func (pr *perfRunner) runLoop(tc TestCase) error {
	testName := tc.Name()
	workerID := tc.WorkerID()
	preFixedWorkerID := fmt.Sprintf("%s%d", workerPrefix, workerID)

	loop := 0

	for {
		select {
		case <-pr.pauseRequests[workerID]:
			// Worker received pause request - send acknowledgment
			pr.pauseAcks[workerID] <- struct{}{}

			// Wait for resume signal
			<-pr.resumeSignals[workerID]

			// Continue to next iteration of loop
			continue
		case <-pr.bfr:
			var actionsCompleted int

			// Worker sends its task
			hist, histErr := perfTestDurationHistogram.GetMetricWith(prometheus.Labels{
				"test": string(testName),
			})

			if histErr != nil {
				log.Errorf("Error retrieving histogram: %s", histErr)
			}

			startTime := time.Now()

			type ActionResponse struct {
				transactionID string
				err           error
			}

			actionResponses := make(chan *ActionResponse, tc.ActionsPerLoop())

			var sentTime time.Time
			var submissionSecondsPerLoop float64
			var eventReceivingSecondsPerLoop float64
			transactionIDs := make([]string, 0)

			pendingActions := 0
			for actionsCompleted = 0; actionsCompleted < tc.ActionsPerLoop(); actionsCompleted++ {

				if pr.allActionsComplete() {
					break
				}
				actionCount := actionsCompleted
				pendingActions++
				go func() {
					transactionID, err := tc.RunOnce(actionCount)
					log.Debugf("%d --> %s action %d sent after %f seconds", workerID, testName, actionCount, time.Since(startTime).Seconds())
					actionResponses <- &ActionResponse{
						transactionID: transactionID,
						err:           err,
					}
				}()
			}
			resultCount := 0
			for pendingActions > 0 {
				aResponse := <-actionResponses
				pendingActions--
				resultCount++
				if aResponse.err != nil {
					if pr.cfg.DelinquentAction == conf.DelinquentActionExit {
						return aResponse.err
					} else {
						log.Errorf("Worker %d error running job (logging but continuing): %s", workerID, aResponse.err)
					}
				} else {
					transactionIDs = append(transactionIDs, aResponse.transactionID)
					pr.workerIDMap.Store(aResponse.transactionID, tc.WorkerID())
					pr.markTestInFlight(tc, aResponse.transactionID)
					log.Debugf("%d --> %s Sent transaction ID: %s", workerID, testName, aResponse.transactionID)
					totalActionsCounter.Inc()
				}
			}
			// if we've reached the expected amount of metadata calls then stop
			if resultCount == tc.ActionsPerLoop() {
				submissionDurationPerLoop := time.Since(startTime)
				pr.sendTime.Record(submissionDurationPerLoop)
				submissionSecondsPerLoop = submissionDurationPerLoop.Seconds()
				sentTime = time.Now()
				log.Debugf("%d --> %s All actions sent %d after %f seconds", workerID, testName, resultCount, submissionSecondsPerLoop)

				pr.endSendTime = time.Now().Unix()
			}
			// Wait for worker to confirm each message before proceeding to next task
			if !pr.cfg.NoWaitSubmission {
				for j := 0; j < actionsCompleted; j++ {
					<-pr.wsReceivers[preFixedWorkerID]
					if len(transactionIDs) > 0 {
						nextTransactionID := transactionIDs[0]
						transactionIDs = transactionIDs[1:]
						pr.stopTrackingRequest(nextTransactionID)
					}
				}
			}
			totalDurationPerLoop := time.Since(startTime)
			pr.totalTime.Record(totalDurationPerLoop)
			secondsPerLoop := totalDurationPerLoop.Seconds()

			eventReceivingDurationPerLoop := time.Since(sentTime)
			eventReceivingSecondsPerLoop = eventReceivingDurationPerLoop.Seconds()
			pr.receiveTime.Record(totalDurationPerLoop)

			total := submissionSecondsPerLoop + eventReceivingSecondsPerLoop
			subPortion := int((submissionSecondsPerLoop / total) * 100)
			envPortion := int((eventReceivingSecondsPerLoop / total) * 100)
			log.Infof("%d <-- %s Finished (loop=%d), submission time: %f s, event receive time: %f s. Ratio (%d/%d) after %f seconds", workerID, testName, loop, submissionSecondsPerLoop, eventReceivingSecondsPerLoop, subPortion, envPortion, secondsPerLoop)

			if histErr == nil {
				log.Debugf("%d <-- %s Emmiting (loop=%d) after %f seconds", workerID, testName, loop, secondsPerLoop)

				hist.Observe(secondsPerLoop)
			}
			loop++
		case <-pr.ctx.Done():
			return nil
		}
	}
}

func (pr *perfRunner) detectDelinquentMsgs() bool {
	delinquentMsgs := make(map[string]time.Time)
	pr.msgTimeMap.Range(func(k, v interface{}) bool {
		trackingID := k.(string)
		inflight := v.(*inflightTest)
		if time.Since(inflight.time).Seconds() > pr.cfg.MaxTimePerAction.Seconds() {
			delinquentMsgs[trackingID] = inflight.time
		}
		return true
	})

	dw, err := json.MarshalIndent(delinquentMsgs, "", "  ")
	if err != nil {
		log.Errorf("Error printing delinquent messages: %s", err)
		return len(delinquentMsgs) > 0
	}

	if len(delinquentMsgs) > 0 {
		log.Warnf("Delinquent Messages:\n%s", string(dw))
	}

	return len(delinquentMsgs) > 0
}

func (pr *perfRunner) markTestInFlight(tc TestCase, transactionID string) {
	if len(transactionID) > 0 {
		pr.msgTimeMap.Store(transactionID, &inflightTest{
			testCase: tc,
			time:     time.Now(),
		})
	}
}

func (pr *perfRunner) recordCompletedAction() {
	if pr.ramping() {
		_ = atomic.AddInt64(&pr.summary.rampSummary, 1)
	} else {
		_ = atomic.AddInt64(&pr.summary.totalSummary, 1)
	}
}

func (pr *perfRunner) stopTrackingRequest(transactionID string) {
	log.Debugf("Deleting tracking request: %s", transactionID)
	pr.msgTimeMap.Delete(transactionID)
}

func (pr *perfRunner) subscribeToPublicContractListener() error {
	// Create receipt listener for public transactions, starting from latest sequence
	pr.listenerName = "publiclistener"
	listenerName := pr.listenerName

	var latestSequence *uint64
	qb := query.NewQueryBuilder().Equal("type", "public").Sort("-sequence").Limit(1)
	receipts, err := pr.httpClients[0].PTX().QueryTransactionReceipts(pr.ctx, qb.Query())
	if err == nil && len(receipts) > 0 {
		seq := receipts[0].Sequence
		latestSequence = &seq
		log.Infof("Found latest sequence: %d, will start listener from sequence above this", seq)
	} else {
		log.Info("No existing receipts found, starting listener from beginning")
	}

	_, err = pr.httpClients[0].PTX().DeleteReceiptListener(pr.ctx, listenerName)
	if err != nil {
		log.Debugf("No existing listener to delete (or delete failed): %v", err)
	}

	txType := pldapi.TransactionTypePublic.Enum()
	_, err = pr.httpClients[0].PTX().CreateReceiptListener(pr.ctx, &pldapi.TransactionReceiptListener{
		Name: listenerName,
		Filters: pldapi.TransactionReceiptFilters{
			Type:          &txType,
			SequenceAbove: latestSequence,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create receipt listener: %w", err)
	}

	// Subscribe to receipts
	sub, err := pr.wsClient.PTX().SubscribeReceipts(pr.ctx, listenerName)
	if err != nil {
		return fmt.Errorf("failed to subscribe to public receipts: %w", err)
	}

	pr.subscriptions = append(pr.subscriptions, sub)
	go pr.batchEventLoop(sub)

	return nil
}

func (pr *perfRunner) IsDaemon() bool {
	return pr.cfg.Daemon
}

func (pr *perfRunner) getIdempotencyKey(workerId int, iteration int) string {
	// Left pad worker ID to 5 digits (supporting up to 99,999 workers)
	workerIdStr := fmt.Sprintf("%05d", workerId)
	// Left pad iteration ID to 9 digits (supporting up to 999,999,999 iterations)
	iterationIdStr := fmt.Sprintf("%09d", iteration)
	return fmt.Sprintf("%v-%s-%s-%s", pr.startTime, workerIdStr, iterationIdStr, uuid.New())
}

func (pr *perfRunner) calculateCurrentTps(logValue bool) float64 {
	// If we're still ramping, give the current rate during the ramp
	// If we're done ramping, calculate TPS from the end of the ramp onward
	var startTime int64
	var measuredActions int64
	if pr.ramping() {
		measuredActions = pr.summary.rampSummary
		startTime = pr.startRampTime
	} else {
		measuredActions = pr.summary.totalSummary
		startTime = pr.startTime
	}
	duration := time.Since(time.Unix(startTime, 0)).Seconds()
	currentTps := float64(measuredActions) / duration
	if logValue {
		log.Infof("Current TPS: %v Measured Actions: %v Duration: %v", currentTps, measuredActions, duration)
	}
	return currentTps
}

func (pr *perfRunner) ramping() bool {
	if time.Now().Before(time.Unix(pr.endRampTime, 0)) {
		return true
	}
	return false
}
