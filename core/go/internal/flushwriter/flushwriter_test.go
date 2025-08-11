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
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testWritable struct {
	input string
}

type testResult struct {
	output string
}

func (tw *testWritable) WriteKey() string {
	return tw.input
}

var testDefaults = &pldconf.FlushWriterConfig{
	WorkerCount:  confutil.P(1),
	BatchTimeout: confutil.P("100m"), // tests set this if they need it
	BatchMaxSize: confutil.P(1),
}

func newTestWriter(t *testing.T, conf *pldconf.FlushWriterConfig, handler BatchHandler[*testWritable, *testResult]) (context.Context, *writer[*testWritable, *testResult], sqlmock.Sqlmock, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	w := NewWriter(ctx, handler, p.P, conf, testDefaults)
	w.Start()
	return ctx, w.(*writer[*testWritable, *testResult]), p.Mock, func() {
		panicked := recover()
		if panicked != nil {
			panic(panicked)
		}
		require.NoError(t, p.Mock.ExpectationsWereMet())
		w.Shutdown()
		cancelCtx()
	}
}

func writeTestOps(ctx context.Context, w *writer[*testWritable, *testResult], count int) []Operation[*testWritable, *testResult] {
	ops := make([]Operation[*testWritable, *testResult], count)
	for i := 0; i < len(ops); i++ {
		tw := &testWritable{input: fmt.Sprintf("write_%.3d", i)}
		if i < (len(ops) - 1) {
			ops[i] = w.Queue(ctx, tw)
		} else {
			ops[i] = w.QueueWithFlush(ctx, tw)
		}
	}
	return ops
}

func TestSuccessfulWriteBatch(t *testing.T) {
	dbTXResult := make(chan error, 1)

	ctx, w, mdb, done := newTestWriter(t, &pldconf.FlushWriterConfig{
		BatchMaxSize: confutil.P(1000),
	},
		func(ctx context.Context, dbTX persistence.DBTX, values []*testWritable) ([]Result[*testResult], error) {
			results := make([]Result[*testResult], len(values))
			for i, v := range values {
				results[i] = Result[*testResult]{R: &testResult{output: v.input}}
			}
			dbTX.AddFinalizer(func(ctx context.Context, err error) { dbTXResult <- err })
			return results, nil
		},
	)
	defer done()

	mdb.ExpectBegin()
	mdb.ExpectCommit()

	ops := writeTestOps(ctx, w, 11)
	for i, op := range ops {
		r, err := op.WaitFlushed(ctx)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("write_%.3d", i), r.output)
	}

	require.Nil(t, <-dbTXResult)
}

func TestBatchTimeout(t *testing.T) {
	dbTXResult := make(chan error, 1)

	ctx, w, mdb, done := newTestWriter(t, &pldconf.FlushWriterConfig{
		BatchMaxSize: confutil.P(1000),
		BatchTimeout: confutil.P("10ms"),
	},
		func(ctx context.Context, dbTX persistence.DBTX, values []*testWritable) ([]Result[*testResult], error) {
			results := make([]Result[*testResult], len(values))
			for i, v := range values {
				results[i] = Result[*testResult]{R: &testResult{output: v.input}}
			}
			dbTX.AddFinalizer(func(ctx context.Context, err error) { dbTXResult <- err })
			return results, nil
		},
	)
	defer done()

	mdb.ExpectBegin()
	mdb.ExpectCommit()

	op := w.Queue(ctx, &testWritable{input: "for timeout"})
	<-op.Flushed()

	require.Nil(t, <-dbTXResult)

}

func TestShutdownNowInBatchWait(t *testing.T) {
	ctx, w, _, done := newTestWriter(t, &pldconf.FlushWriterConfig{
		BatchMaxSize: confutil.P(1000),
		BatchTimeout: confutil.P("10ms"),
	}, nil)
	defer done()

	_ = w.Queue(ctx, &testWritable{input: "doomed to fail"})
	w.ShutdownNow()
}

func TestBadResult(t *testing.T) {
	dbTXResult := make(chan error, 1)

	ctx, w, mdb, done := newTestWriter(t, &pldconf.FlushWriterConfig{
		BatchMaxSize: confutil.P(1000),
	},
		func(ctx context.Context, dbTX persistence.DBTX, values []*testWritable) ([]Result[*testResult], error) {
			dbTX.AddFinalizer(func(ctx context.Context, err error) { dbTXResult <- err })
			return make([]Result[*testResult], len(values)-1), nil
		})
	defer done()

	mdb.ExpectBegin()
	mdb.ExpectCommit()

	ops := writeTestOps(ctx, w, 11)
	for _, op := range ops {
		_, err := op.WaitFlushed(ctx)
		assert.Regexp(t, "PD012301", err)
	}

	// Note we still dispatch the nil error regarding the DB transaction to the DB result callback
	require.NoError(t, <-dbTXResult)
}

func TestBadOp(t *testing.T) {
	ctx, w, _, done := newTestWriter(t, &pldconf.FlushWriterConfig{
		BatchMaxSize: confutil.P(1000),
	}, nil)
	defer done()

	op := w.Queue(ctx, &testWritable{})
	_, err := op.WaitFlushed(ctx)
	require.Regexp(t, "PD012302", err)
}

func TestIndividualError(t *testing.T) {
	dbTXResult := make(chan error, 1)

	ctx, w, mdb, done := newTestWriter(t, &pldconf.FlushWriterConfig{
		BatchMaxSize: confutil.P(1000),
	},
		func(ctx context.Context, dbTX persistence.DBTX, values []*testWritable) ([]Result[*testResult], error) {
			dbTX.AddFinalizer(func(ctx context.Context, err error) { dbTXResult <- err })
			return []Result[*testResult]{
				{R: &testResult{output: "worked"}},
				{Err: fmt.Errorf("failed")},
			}, nil
		},
	)
	defer done()

	mdb.ExpectBegin()
	mdb.ExpectCommit()

	ops := writeTestOps(ctx, w, 2)
	r, err := ops[0].WaitFlushed(ctx)
	require.NoError(t, err)
	assert.Equal(t, "worked", r.output)
	r, err = ops[1].WaitFlushed(ctx)
	require.EqualError(t, err, "failed")
	assert.Nil(t, r)

	require.NoError(t, <-dbTXResult)

}

func TestWaitFlushTimeout(t *testing.T) {
	ctx, w, _, done := newTestWriter(t, &pldconf.FlushWriterConfig{
		BatchMaxSize: confutil.P(1000),
	},
		func(ctx context.Context, dbTX persistence.DBTX, values []*testWritable) ([]Result[*testResult], error) {
			return nil, fmt.Errorf("should not make it back to call")
		},
	)
	done()
	ops := writeTestOps(ctx, w, 11)
	for _, op := range ops {
		_, err := op.WaitFlushed(ctx)
		assert.Regexp(t, "PD010301|PD012300", err)
	}

}

func TestFailedWriteBatch(t *testing.T) {
	ctx, w, mdb, done := newTestWriter(t, &pldconf.FlushWriterConfig{
		BatchMaxSize: confutil.P(1000),
	},
		func(ctx context.Context, dbTX persistence.DBTX, values []*testWritable) ([]Result[*testResult], error) {
			return nil, fmt.Errorf("pop")
		},
	)
	defer done()

	mdb.ExpectBegin()
	mdb.ExpectRollback()

	ops := writeTestOps(ctx, w, 11)
	for _, op := range ops {
		r, err := op.WaitFlushed(ctx)
		assert.True(t, r == nil) // deliberate equality test here
		assert.Regexp(t, "pop", err)
	}
}

func TestShutdownNowOverride(t *testing.T) {
	tw := &writer[*testWritable, *testResult]{
		workQueues: []chan *op[*testWritable, *testResult]{
			make(chan *op[*testWritable, *testResult]),
		},
		workersDone: []chan struct{}{
			make(chan struct{}),
		},
	}
	tw.bgCtx, tw.cancelCtx = context.WithCancel(context.Background())
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		tw.Shutdown()
	}()
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		tw.ShutdownNow()
	}()
	// Simulate worker shutdown only when the context closes
	<-tw.bgCtx.Done()
	close(tw.workersDone[0])
	// Wait until both complete
	wg.Wait()
}

func TestShutdownAfterClose(t *testing.T) {
	tw := &writer[*testWritable, *testResult]{
		workQueues: []chan *op[*testWritable, *testResult]{
			make(chan *op[*testWritable, *testResult]),
		},
		workersDone: []chan struct{}{
			make(chan struct{}),
		},
	}
	close(tw.workersDone[0])
	tw.bgCtx, tw.cancelCtx = context.WithCancel(context.Background())
	tw.cancelCtx()
	tw.Shutdown()
}
