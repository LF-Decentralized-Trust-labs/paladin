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

package statedistribution

import (
	"context"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/flushwriter"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

type receivedStateWriterNoResult struct{}
type receivedStateWriteOperation struct {
	DomainName      string
	ContractAddress tktypes.EthAddress
	SchemaID        tktypes.Bytes32
	StateDataJson   tktypes.RawJSON
}

type receivedStateWriter struct {
	flushWriter  flushwriter.Writer[*receivedStateWriteOperation, *receivedStateWriterNoResult]
	stateManager components.StateManager
}

func NewReceivedStateWriter(ctx context.Context, stateManager components.StateManager, persistence persistence.Persistence, conf *pldconf.FlushWriterConfig) *receivedStateWriter {
	rsw := &receivedStateWriter{
		stateManager: stateManager,
	}
	rsw.flushWriter = flushwriter.NewWriter(ctx, rsw.runBatch, persistence, conf, &pldconf.StateDistributerWriterConfigDefaults)
	return rsw
}

func (wo *receivedStateWriteOperation) WriteKey() string {
	return wo.DomainName
}

func (rsw *receivedStateWriter) runBatch(ctx context.Context, tx *gorm.DB, values []*receivedStateWriteOperation) ([]flushwriter.Result[*receivedStateWriterNoResult], error) {
	log.L(ctx).Debugf("receivedStateWriter:runBatch %d acknowledgements", len(values))

	if len(values) == 0 {
		return nil, nil
	}

	stateUpserts := make([]*components.StateUpsertOutsideContext, len(values))
	for i, receivedStateWriteOperation := range values {
		stateUpserts[i] = &components.StateUpsertOutsideContext{
			ContractAddress: receivedStateWriteOperation.ContractAddress,
			SchemaID:        receivedStateWriteOperation.SchemaID,
			Data:            receivedStateWriteOperation.StateDataJson,
		}
	}

	_, err := rsw.stateManager.WriteReceivedStates(ctx, tx, values[0].DomainName, stateUpserts)

	if err != nil {
		log.L(ctx).Errorf("Error writing received states: %s", err)
	}

	// We don't actually provide any result, so just build an array of nil results
	return make([]flushwriter.Result[*receivedStateWriterNoResult], len(values)), err

}

func (rsw *receivedStateWriter) Start() {
	rsw.flushWriter.Start()
}

func (rsw *receivedStateWriter) QueueAndWait(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, schemaID tktypes.Bytes32, stateDataJson tktypes.RawJSON) error {
	log.L(ctx).Debugf("receivedStateWriter:QueueAndWait %s %s %s", domainName, contractAddress, schemaID)
	op := rsw.flushWriter.Queue(ctx, &receivedStateWriteOperation{
		DomainName:      domainName,
		ContractAddress: contractAddress,
		SchemaID:        schemaID,
		StateDataJson:   stateDataJson,
	})
	_, err := op.WaitFlushed(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error waiting for state distribution write: %s", err)
	}
	return err
}
