// Copyright © 2023 Kaleido, Inc.
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

package statestore

import (
	"context"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestExecuteBatchOpsInsertBadOp(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	txOp := &writeOperation{
		id:   types.ShortID(),
		done: make(chan error, 1),
	}
	ss.writer.queue(ctx, txOp)
	err := txOp.flush(ctx)
	assert.Regexp(t, "PD010105", err)
}

func TestStopDoneWorker(t *testing.T) {
	tw := &stateWriter{
		workersDone: []chan struct{}{
			make(chan struct{}),
		},
	}
	tw.bgCtx, tw.cancelCtx = context.WithCancel(context.Background())
	close(tw.workersDone[0])
	tw.stop()
}

func TestStopDoneCtx(t *testing.T) {
	tw := &stateWriter{
		workersDone: []chan struct{}{
			make(chan struct{}, 1),
		},
	}
	tw.bgCtx, tw.cancelCtx = context.WithCancel(context.Background())
	tw.cancelCtx()
	go func() {
		time.Sleep(10 * time.Millisecond)
		tw.workersDone[0] <- struct{}{}
	}()
	tw.stop()
}
