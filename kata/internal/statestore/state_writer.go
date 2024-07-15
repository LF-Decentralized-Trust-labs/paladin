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

package statestore

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
)

type stateUpdateType int

const (
	updateWriteState stateUpdateType = iota
	updateWriteStateUpdate
)

type writeOperation struct {
	id           string
	domain       string
	done         chan error
	isShutdown   bool
	states       []*State
	schemas      []*SchemaEntity
	stateUpdates []*StateUpdate
}

type stateWriter struct {
	ss           *stateStore
	bgCtx        context.Context
	cancelCtx    context.CancelFunc
	batchTimeout time.Duration
	batchMaxSize int
	workerCount  uint32
	workQueues   []chan *writeOperation
	workersDone  []chan struct{}
}

type stateWriterBatch struct {
	id             string
	opened         time.Time
	ops            []*writeOperation
	timeoutContext context.Context
	timeoutCancel  func()
}

func newStateWriter(bgCtx context.Context, ss *stateStore, conf *StateWriterConfig) *stateWriter {
	workerCount := confutil.IntMin(conf.WorkerCount, 1, *StateWriterConfigDefaults.WorkerCount)
	batchMaxSize := confutil.IntMin(conf.BatchMaxSize, 1, *StateWriterConfigDefaults.BatchMaxSize)
	batchTimeout := confutil.Duration(conf.BatchTimeout, *StateWriterConfigDefaults.BatchTimeout)
	sw := &stateWriter{
		ss:           ss,
		workerCount:  (uint32)(workerCount),
		batchTimeout: batchTimeout,
		batchMaxSize: batchMaxSize,
		workersDone:  make([]chan struct{}, workerCount),
		workQueues:   make([]chan *writeOperation, workerCount),
	}
	sw.bgCtx, sw.cancelCtx = context.WithCancel(bgCtx)
	for i := 0; i < workerCount; i++ {
		sw.workersDone[i] = make(chan struct{})
		sw.workQueues[i] = make(chan *writeOperation, batchMaxSize)
		go sw.worker(i)
	}
	return sw
}

func (sw *stateWriter) newWriteOp(domain string) *writeOperation {
	return &writeOperation{
		id:     types.ShortID(),
		domain: domain,
		done:   make(chan error, 1), // 1 slot to ensure we don't block the writer
	}
}

func (op *writeOperation) flush(ctx context.Context) error {
	select {
	case err := <-op.done:
		log.L(ctx).Debugf("Flushed write operation %s (err=%v)", op.id, err)
		return err
	case <-ctx.Done():
		return i18n.NewError(ctx, i18n.MsgContextCanceled)
	}
}

func (sw *stateWriter) queue(ctx context.Context, op *writeOperation) {
	// All insert/nonce-allocation requests for the same domain go to the same worker
	// currently. This allows assertions to be made between threads writing schemas,
	// threads writing state updates, and threads writing new states.
	if op.domain == "" {
		op.done <- i18n.NewError(ctx, msgs.MsgStateOpInvalid)
		return
	}
	h := fnv.New32a() // simple non-cryptographic hash algo
	_, _ = h.Write([]byte(op.domain))
	routine := h.Sum32() % sw.workerCount
	log.L(ctx).Debugf("Queuing write operation %s to worker state_writer_%.4d", op.id, routine)
	select {
	case sw.workQueues[routine] <- op: // it's queued
	case <-ctx.Done(): // timeout of caller context
		// Just return, as they are giving up on the request so there's no need to queue it
		// If they flush they will get an error
	case <-sw.bgCtx.Done(): // shutdown
		// Push an error back to the operator before we return (note we allocate a slot to make this safe)
		op.done <- i18n.NewError(ctx, msgs.MsgStateManagerQuiescing)
	}
}

func (sw *stateWriter) worker(i int) {
	defer close(sw.workersDone[i])
	workerID := fmt.Sprintf("state_writer_%.4d", i)
	ctx := log.WithLogField(sw.bgCtx, "job", workerID)
	l := log.L(ctx)
	var batch *stateWriterBatch
	batchCount := 0
	workQueue := sw.workQueues[i]
	var shutdownRequest *writeOperation
	for shutdownRequest == nil {
		var timeoutContext context.Context
		var timedOut bool
		if batch != nil {
			timeoutContext = batch.timeoutContext
		} else {
			timeoutContext = ctx
		}
		select {
		case op := <-workQueue:
			if op.isShutdown {
				// flush out the queue
				shutdownRequest = op
				timedOut = true
				break
			}
			if batch == nil {
				batch = &stateWriterBatch{
					id:     fmt.Sprintf("%.4d_%.9d", i, batchCount),
					opened: time.Now(),
				}
				batch.timeoutContext, batch.timeoutCancel = context.WithTimeout(ctx, sw.batchTimeout)
				batchCount++
			}
			batch.ops = append(batch.ops, op)
			l.Debugf("Added write operation %s to batch %s (len=%d)", op.id, batch.id, len(batch.ops))
		case <-timeoutContext.Done():
			timedOut = true
			select {
			case <-ctx.Done():
				l.Debugf("State writer ending")
				return
			default:
			}
		}

		if batch != nil && (timedOut || (len(batch.ops) >= sw.batchMaxSize)) {
			batch.timeoutCancel()
			l.Debugf("Running batch %s (len=%d,timeout=%t,age=%dms)", batch.id, len(batch.ops), timedOut, time.Since(batch.opened).Milliseconds())
			sw.runBatch(ctx, batch)
			batch = nil
		}

		if shutdownRequest != nil {
			close(shutdownRequest.done)
		}
	}
}

func (sw *stateWriter) runBatch(ctx context.Context, b *stateWriterBatch) {

	// Build lists of things to insert (we are insert only)
	var schemas []*SchemaEntity
	var states []*State
	var stateUpdates []*StateUpdate
	for _, op := range b.ops {
		if len(op.schemas) > 0 {
			schemas = append(schemas, op.schemas...)
		}
		if len(op.states) > 0 {
			states = append(states, op.states...)
		}
		if len(op.stateUpdates) > 0 {
			stateUpdates = append(stateUpdates, op.stateUpdates...)
		}
	}
	log.L(ctx).Debugf("Writing state batch schemas=%d states=%d stateUpdates=%d", len(schemas), len(states), len(stateUpdates))

	err := sw.ss.p.DB().Transaction(func(tx *gorm.DB) (err error) {
		if len(schemas) > 0 {
			err = tx.
				Table("schemas").
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(schemas).
				Error
		}
		if err == nil && len(states) > 0 {
			err = tx.
				Table("states").
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(states).
				Error
		}
		if err == nil && len(stateUpdates) > 0 {
			err = tx.
				Table("state_updates").
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(stateUpdates).
				Error
		}
		return err
	})

	// Mark all the ops complete - for good or bad
	for _, op := range b.ops {
		op.done <- err
	}
}

func (sw *stateWriter) stop() {
	for i, workerDone := range sw.workersDone {
		select {
		case <-workerDone:
		case <-sw.bgCtx.Done():
		default:
			// Quiesce the worker
			shutdownOp := &writeOperation{
				isShutdown: true,
				done:       make(chan error),
			}
			sw.workQueues[i] <- shutdownOp
			<-shutdownOp.done
		}
		<-workerDone
	}
	sw.cancelCtx()
}
