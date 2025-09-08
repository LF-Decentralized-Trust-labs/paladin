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
	"fmt"

	"github.com/LF-Decentralized-Trust-labs/paladin/perf/internal/conf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type publicContract struct {
	testBase
}

func newPublicContractTestWorker(pr *perfRunner, workerID int, actionsPerLoop int) TestCase {
	return &publicContract{
		testBase: testBase{
			pr:             pr,
			workerID:       workerID,
			actionsPerLoop: actionsPerLoop,
		},
	}
}

func (tc *publicContract) Name() conf.TestName {
	return conf.PerfTestPublicContract
}

func (tc *publicContract) RunOnce(iterationCount int) (string, error) {
	// This will always be the ABI reference for simple storage - can disregard error when hardcoded to a valid value
	abiRef := pldtypes.MustParseBytes32("0x23dbc09b901a3bf265a44b60ca7337eeba63f506ddd8ed77ac1505a52a2c5d15")
	to := pldtypes.MustEthAddress(tc.pr.cfg.ContractOptions.Address)
	result, err := tc.pr.httpClient.PTX().SendTransaction(tc.pr.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:         pldapi.TransactionTypePublic.Enum(),
			ABIReference: &abiRef,
			Function:     "set",
			To:           to,
			// This test is more valuable if it uses different signing keys, otherwise it only exercises
			// a single transaction orchestrator. This approach works when using the default paladin
			// wallet, but may require additional configuration if testing with an external wallet
			From:           fmt.Sprintf("test%d", tc.workerID),
			Data:           pldtypes.RawJSON(fmt.Sprintf("[%d]", tc.workerID)),
			IdempotencyKey: tc.pr.getIdempotencyKey(tc.workerID, iterationCount),
		},
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprint(result), nil
}
