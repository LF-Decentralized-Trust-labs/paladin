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

package flushwriter

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
)

type Writeable[R any] interface {
	// Default mode of operation of this utility is that the objects must expose a
	// field (in string format) that can be used to determine which writer will
	// be used. All writes on the same value are guaranteed to go to the same writer.
	WriteKey() string
}

type Operation[T Writeable[R], R any] interface {
	// Flush blocks for the operation to complete, or a failure.
	// - if the writer shuts down, that will be a failure
	// - if the supplied context is cancelled,
	WaitFlushed(ctx context.Context) (R, error)

	// Flushed returns the flush channel directly.
	// Use one or the other of Flush/Flushed - if you use both
	// the behavior if undefined
	Flushed() <-chan Result[R]
}

// The writes are generally insert operations with OnConflict.
// However, more processing can be included and unique errors can be returned
// within the Result object, that do not fail the whole batch.
// - Result.Err - transaction completes, this one operation gets an error in their result
// - error - whole batch rolls back, and error is delivered in all Result[].Err for batch
//
// Note: If you perform a failed DB operation, then the whole DB transaction
// will rollback even if you don't return an error.
type BatchHandler[T Writeable[R], R any] func(ctx context.Context, dbTX persistence.DBTX, values []T) ([]Result[R], error)

type Writer[T Writeable[R], R any] interface {
	Start()                                                      // the routines do not run until this is called
	Queue(ctx context.Context, value T) Operation[T, R]          // add an operation to be executed
	QueueWithFlush(ctx context.Context, value T) Operation[T, R] // USE WITH CARE - causes write that picks up this operation to close its batch as soon as it picks this up
	Shutdown()                                                   // waits for all in process work to complete, then shuts down
	ShutdownNow()                                                // cancels the context to interrupt all write operations
}

type Result[R any] struct {
	Err                error
	R                  R
	DBTXResultCallback func(error)
}

type op[T Writeable[R], R any] struct {
	id         string
	writeKey   string
	flush      bool
	isShutdown bool
	done       chan Result[R]
	value      T
}

type writer[T Writeable[R], R any] struct {
	bgCtx        context.Context
	cancelCtx    context.CancelFunc
	p            persistence.Persistence
	handler      BatchHandler[T, R]
	writerId     string
	batchTimeout time.Duration
	batchMaxSize int
	workerCount  int
	workQueues   []chan *op[T, R]
	workersDone  []chan struct{}
}

type batch[T Writeable[R], R any] struct {
	id             string
	opened         time.Time
	ops            []*op[T, R]
	timeoutContext context.Context
	timeoutCancel  func()
}

func NewWriter[T Writeable[R], R any](
	bgCtx context.Context,
	handler BatchHandler[T, R],
	p persistence.Persistence,
	conf *pldconf.FlushWriterConfig,
	defaults *pldconf.FlushWriterConfig,
) Writer[T, R] {
	workerCount := confutil.IntMin(conf.WorkerCount, 1, *defaults.WorkerCount)
	batchMaxSize := confutil.IntMin(conf.BatchMaxSize, 1, *defaults.BatchMaxSize)
	batchTimeout := confutil.DurationMin(conf.BatchTimeout, 0, *defaults.BatchTimeout)
	w := &writer[T, R]{
		p:            p,
		writerId:     pldtypes.ShortID(), // so logs distinguish these writers from any others
		handler:      handler,
		workerCount:  workerCount,
		batchTimeout: batchTimeout,
		batchMaxSize: batchMaxSize,
	}
	w.bgCtx, w.cancelCtx = context.WithCancel(bgCtx)
	return w
}

func (w *writer[T, R]) Start() {
	log.L(w.bgCtx).Debugf("Starting %d workers for writer %s", w.workerCount, w.writerId)
	w.workersDone = make([]chan struct{}, w.workerCount)
	w.workQueues = make([]chan *op[T, R], w.workerCount)
	for i := 0; i < w.workerCount; i++ {
		w.workersDone[i] = make(chan struct{})
		w.workQueues[i] = make(chan *op[T, R], w.batchMaxSize)
		go w.worker(i)
	}
}

func (w *writer[T, R]) Queue(ctx context.Context, value T) Operation[T, R] {
	return w.queue(ctx, value, false)
}

func (w *writer[T, R]) QueueWithFlush(ctx context.Context, value T) Operation[T, R] {
	return w.queue(ctx, value, true)
}

func (op *op[T, R]) WaitFlushed(ctx context.Context) (R, error) {
	select {
	case r := <-op.done:
		log.L(ctx).Debugf("Flushed write operation %s (key=%s,err=%v)", op.id, op.writeKey, r.Err)
		return r.R, r.Err
	case <-ctx.Done():
		return *(new(R)), i18n.NewError(ctx, msgs.MsgContextCanceled)
	}
}

func (op *op[T, R]) Flushed() <-chan Result[R] {
	return op.done
}

func (w *writer[T, R]) queue(ctx context.Context, value T, flush bool) *op[T, R] {
	op := &op[T, R]{
		id:       pldtypes.ShortID(),
		writeKey: value.WriteKey(),
		value:    value,
		flush:    flush,
		done:     make(chan Result[R], 1), // 1 slot to ensure we don't block the writer
	}
	if op.writeKey == "" {
		op.done <- Result[R]{Err: i18n.NewError(ctx, msgs.MsgFlushWriterOpInvalid)}
		return op
	}

	// All requests on the same key go to the same worker.
	// This allows assertions to be made between threads writing schemas,
	// threads writing state updates, and threads writing new states.
	h := fnv.New32a() // simple non-cryptographic hash algo
	_, _ = h.Write([]byte(op.writeKey))
	routine := h.Sum32() % uint32(w.workerCount)
	log.L(ctx).Debugf("Queuing write operation %s to writer_%s_%.4d", op.id, w.writerId, routine)
	select {
	case w.workQueues[routine] <- op: // it's queued
	case <-ctx.Done(): // timeout of caller context
		// Just return, as they are giving up on the request so there's no need to queue it
		// If they call WaitFlush they will get an error (if they wait on the Flushed() channel that's
		// their responsibility to check their context too)
	case <-w.bgCtx.Done(): // shutdown
		// Push an error back to the operator before we return (note we allocate a slot to make this safe)
		op.done <- Result[R]{Err: i18n.NewError(ctx, msgs.MsgFlushWriterQuiescing)}
	}

	return op
}

func (w *writer[T, R]) worker(i int) {
	defer close(w.workersDone[i])
	workerID := fmt.Sprintf("writer_%s_%.4d", w.writerId, i)
	ctx := log.WithLogField(w.bgCtx, "job", workerID)
	l := log.L(ctx)
	var b *batch[T, R]
	batchCount := 0
	workQueue := w.workQueues[i]
	var shutdownRequest *op[T, R]
	for shutdownRequest == nil {
		var timeoutContext context.Context
		var timedOutOrFlush bool
		if b != nil {
			timeoutContext = b.timeoutContext
		} else {
			timeoutContext = ctx
		}
		select {
		case op := <-workQueue:
			if op.isShutdown {
				// flush out the queue
				shutdownRequest = op
				timedOutOrFlush = true
				break
			}
			if op.flush {
				// This op will kick off the batch regardless of how full it is
				timedOutOrFlush = true
			}
			if b == nil {
				b = &batch[T, R]{
					id:     fmt.Sprintf("%.4d_%.9d", i, batchCount),
					opened: time.Now(),
				}
				b.timeoutContext, b.timeoutCancel = context.WithTimeout(ctx, w.batchTimeout)
				batchCount++
			}
			b.ops = append(b.ops, op)
			l.Debugf("Added write operation %s to batch %s (len=%d)", op.id, b.id, len(b.ops))
		case <-timeoutContext.Done():
			timedOutOrFlush = true
			select {
			case <-ctx.Done():
				l.Debugf("Writer ending")
				return
			default:
			}
		}

		if b != nil && (timedOutOrFlush || (len(b.ops) >= w.batchMaxSize)) {
			b.timeoutCancel()
			l.Debugf("Running batch %s (len=%d,timeout=%t,age=%dms)", b.id, len(b.ops), timedOutOrFlush, time.Since(b.opened).Milliseconds())
			w.runBatch(ctx, b)
			b = nil
		}

		if shutdownRequest != nil {
			close(shutdownRequest.done)
		}
	}
}

func (w *writer[T, R]) runBatch(ctx context.Context, b *batch[T, R]) {

	// Build lists of things to insert (we are insert only)
	values := make([]T, len(b.ops))
	keys := make([]string, len(b.ops))
	for i, op := range b.ops {
		values[i] = op.value
		keys[i] = op.writeKey
	}
	log.L(ctx).Debugf("Writing batch count=%d keys=%v", len(keys), keys)

	// We promise to call any registered result callback with the DB Transaction result on all paths.
	var txErr error

	var results []Result[R]
	txErr = w.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		results, err = w.handler(ctx, dbTX, values)
		return err
	})
	err := txErr
	if err != nil {
		log.L(ctx).Errorf("Write batch failed: %s", err)
	} else if len(results) != len(values) {
		log.L(ctx).Errorf("Invalid results (values=%d,results=%d): %+v", len(values), len(results), results)
		err = i18n.NewError(ctx, msgs.MsgFlushWriterInvalidResults)
	}

	// Mark all the ops complete - for good or bad
	for i, op := range b.ops {
		if err != nil {
			op.done <- Result[R]{Err: err}
		} else {
			op.done <- results[i]
		}
	}
}

func (w *writer[T, R]) Shutdown() {
	shutdownOps := make([]*op[T, R], len(w.workersDone))
	for i := range w.workersDone {
		// Quiesce the worker
		shutdownOps[i] = &op[T, R]{
			isShutdown: true,
			done:       make(chan Result[R]),
		}
		select {
		case w.workQueues[i] <- shutdownOps[i]:
		case <-w.bgCtx.Done():
		}
	}
	w.waitForShudownOps(shutdownOps)
	w.cancelCtx()
}

func (w *writer[T, R]) waitForShudownOps(shutdownOps []*op[T, R]) {
	for i, workerDone := range w.workersDone {
		select {
		case <-shutdownOps[i].done:
		case <-w.bgCtx.Done():
		}
		<-workerDone
	}
}

func (w *writer[T, R]) ShutdownNow() {
	w.cancelCtx()
	for _, workerDone := range w.workersDone {
		<-workerDone
	}
}
