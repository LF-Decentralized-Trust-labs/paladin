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

	"github.com/kaleido-io/paladin/kata/pkg/types"
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

func TestFlushTimeout(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	txOp := &writeOperation{
		id:     types.ShortID(),
		done:   make(chan error, 1),
		domain: "domain1",
	}
	ss.writer.queue(ctx, txOp)
	closedCtx, closeCtx := context.WithCancel(ctx)
	closeCtx()
	err := txOp.flush(closedCtx)
	assert.Regexp(t, "PD010301", err)
}

func TestFlushClosed(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	txOp := &writeOperation{
		id:     types.ShortID(),
		done:   make(chan error, 1),
		domain: "domain1",
	}
	ss.writer.cancelCtx()
	<-ss.writer.workersDone[0]
	ss.writer.workQueues[0] = make(chan *writeOperation)
	ss.writer.queue(ctx, txOp)
}

func TestFlushCallerClosed(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	txOp := &writeOperation{
		id:     types.ShortID(),
		done:   make(chan error, 1),
		domain: "domain1",
	}
	ss.writer.cancelCtx()
	<-ss.writer.workersDone[0]
	ss.writer.bgCtx = context.Background()
	ss.writer.workQueues[0] = make(chan *writeOperation)
	closedCtx, closeCtx := context.WithCancel(ctx)
	closeCtx()
	ss.writer.queue(closedCtx, txOp)
}
