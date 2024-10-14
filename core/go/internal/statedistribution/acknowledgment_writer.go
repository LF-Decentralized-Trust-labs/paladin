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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/flushwriter"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type acknowledgementWriterNoResult struct{}
type acknowledgementWriteOperation struct {
	StateDistributionID string
}

type acknowledgementWriter struct {
	flushWriter flushwriter.Writer[*acknowledgementWriteOperation, *acknowledgementWriterNoResult]
}

func NewAcknowledgementWriter(ctx context.Context, persistence persistence.Persistence, conf *pldconf.FlushWriterConfig) *acknowledgementWriter {
	aw := &acknowledgementWriter{}
	aw.flushWriter = flushwriter.NewWriter(ctx, aw.runBatch, persistence, conf, &pldconf.StateDistributerWriterConfigDefaults)
	return aw
}

func (wo *acknowledgementWriteOperation) WriteKey() string {
	//no ordering requirements so just assign a worker at random for each write
	return wo.StateDistributionID
}

type stateDistributionAcknowledgement struct {
	StateDistribution string `json:"stateDistribution" gorm:"column:state_distribution"`
	ID                string `json:"id"                gorm:"column:id"`
}

func (aw *acknowledgementWriter) runBatch(ctx context.Context, tx *gorm.DB, values []*acknowledgementWriteOperation) ([]flushwriter.Result[*acknowledgementWriterNoResult], error) {
	log.L(ctx).Debugf("acknowledgementWriter:runBatch %d acknowledgements", len(values))

	acknowledgements := make([]*stateDistributionAcknowledgement, 0, len(values))
	for _, value := range values {
		acknowledgements = append(acknowledgements, &stateDistributionAcknowledgement{
			StateDistribution: value.StateDistributionID,
			ID:                uuid.New().String(),
		})
	}

	err := tx.
		Table("state_distribution_acknowledgments").
		Clauses(clause.OnConflict{
			DoNothing: true, // immutable
		}).
		Create(acknowledgements).
		Error
	if err != nil {
		log.L(ctx).Errorf("Error persisting state distribution acknowledgements: %s", err)
	}

	// We don't actually provide any result, so just build an array of nil results
	return make([]flushwriter.Result[*acknowledgementWriterNoResult], len(values)), err

}

func (aw *acknowledgementWriter) Start() {
	aw.flushWriter.Start()
}

func (aw *acknowledgementWriter) Queue(ctx context.Context, stateDistributionID string) {
	aw.flushWriter.Queue(ctx, &acknowledgementWriteOperation{
		StateDistributionID: stateDistributionID,
	})
}
