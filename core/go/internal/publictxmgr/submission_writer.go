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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/flushwriter"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/publictxmgr/metrics"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"

	"gorm.io/gorm/clause"
)

type noResult struct{}

type submissionWriter struct {
	flushwriter.Writer[*DBPubTxnSubmission, *noResult]
	metrics metrics.PublicTransactionManagerMetrics
}

func newSubmissionWriter(bgCtx context.Context, p persistence.Persistence, conf *pldconf.PublicTxManagerConfig, metrics metrics.PublicTransactionManagerMetrics) *submissionWriter {
	sw := &submissionWriter{}
	sw.metrics = metrics
	sw.Writer = flushwriter.NewWriter(bgCtx, sw.runBatch, p, &conf.Manager.SubmissionWriter, &pldconf.PublicTxManagerDefaults.Manager.SubmissionWriter)
	return sw
}

func (sw *submissionWriter) runBatch(ctx context.Context, tx persistence.DBTX, values []*DBPubTxnSubmission) ([]flushwriter.Result[*noResult], error) {
	err := tx.DB().
		Table("public_submissions").
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tx_hash"}},
			DoNothing: true, // immutable
		}).
		Create(values).
		Error
	if err != nil {
		return nil, err
	}
	sw.metrics.IncDBSubmittedTransactionsByN(uint64(len(values)))
	// We don't actually provide any result, so just build an array of nil results
	return make([]flushwriter.Result[*noResult], len(values)), err
}
