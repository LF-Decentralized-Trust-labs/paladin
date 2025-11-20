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
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	log "github.com/sirupsen/logrus"
)

type privateTransactionNodeRestart struct {
	testBase
	privacyGroupID  *pldtypes.HexBytes
	contractAddress *pldtypes.EthAddress
	httpClients     []pldclient.PaladinClient
	random          *rand.Rand
}

func newPrivateTransactionNodeRestartTestWorker(pr *perfRunner, workerID int, actionsPerLoop int, privacyGroupID *pldtypes.HexBytes, contractAddress *pldtypes.EthAddress, httpClients []pldclient.PaladinClient) TestCase {
	return &privateTransactionNodeRestart{
		testBase: testBase{
			pr:             pr,
			workerID:       workerID,
			actionsPerLoop: actionsPerLoop,
		},
		privacyGroupID:  privacyGroupID,
		contractAddress: contractAddress,
		httpClients:     httpClients,
		random:          rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID))),
	}
}

func (tc *privateTransactionNodeRestart) Name() conf.TestName {
	return conf.PerfTestPrivateTransactionNodeRestart
}

func (tc *privateTransactionNodeRestart) RunOnce(iterationCount int) (string, error) {
	// Randomly select a node from available nodes
	if len(tc.httpClients) == 0 {
		return "", fmt.Errorf("no HTTP clients available")
	}
	nodeIndex := tc.random.Intn(len(tc.httpClients))
	client := tc.httpClients[nodeIndex]

	// Parse the function ABI for the "set" function
	// This matches the SimpleStorage contract set(uint256) function
	setFunctionABI := &abi.Entry{
		Name: "set",
		Type: "function",
		Inputs: abi.ParameterArray{
			{
				Name: "newValue",
				Type: "uint256",
			},
		},
	}

	// Create the input data as JSON
	inputData := map[string]interface{}{
		"newValue": tc.workerID,
	}
	inputJSON, err := json.Marshal(inputData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal input data: %w", err)
	}

	// Submit pente transaction via pgroup_sendTransaction API
	txID, err := client.PrivacyGroups().SendTransaction(tc.pr.ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  *tc.privacyGroupID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			To:       tc.contractAddress,
			Function: setFunctionABI,
			Input:    pldtypes.RawJSON(inputJSON),
		},
		IdempotencyKey: tc.pr.getIdempotencyKey(tc.workerID, iterationCount),
	})
	if err != nil {
		return "", fmt.Errorf("failed to send pente transaction to node %d: %w", nodeIndex, err)
	}

	log.Debugf("Worker %d sent pente transaction %s to node %d", tc.workerID, txID, nodeIndex)
	return txID.String(), nil
}
