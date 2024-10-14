// Copyright © 2024 Kaleido, Inc.
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

package privatetxnstore

import (
	"context"

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/flushwriter"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"

	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

var WriterConfigDefaults = pldconf.FlushWriterConfig{
	WorkerCount:  confutil.P(10),
	BatchTimeout: confutil.P("25ms"),
	BatchMaxSize: confutil.P(100),
}

type PublicTransactionsSubmit func(tx *gorm.DB) (publicTxID []string, err error)

type Store interface {
	//We persist multiple sequences in a single call, one for each signing address
	Start()
	PersistDispatchBatch(ctx context.Context, contractAddress tktypes.EthAddress, dispatchBatch *DispatchBatch, stateDistributions []*statedistribution.StateDistribution) error
	Close()
}

type store struct {
	started bool
	writer  flushwriter.Writer[*dispatchSequenceOperation, *noResult]
}

func NewStore(ctx context.Context, conf *pldconf.FlushWriterConfig, p persistence.Persistence) Store {
	s := &store{}
	s.writer = flushwriter.NewWriter(ctx, s.runBatch, p, conf, &WriterConfigDefaults)
	return s
}

func (s *store) Start() {
	if !s.started {
		s.writer.Start()
	}
}

func (s *store) Close() {
	s.writer.Shutdown()
}
