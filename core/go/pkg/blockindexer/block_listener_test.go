// Copyright © 2022 Kaleido, Inc.
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

package blockindexer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/rpcclient"
	"github.com/kaleido-io/paladin/kata/mocks/rpcbackendmocks"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestBlockListener(t *testing.T) (context.Context, *blockListener, *rpcbackendmocks.WebSocketRPCClient, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	bl, mRPC := newTestBlockListenerConf(t, ctx, &Config{})
	return ctx, bl, mRPC, func() {
		cancelCtx()
		bl.waitClosed()
	}
}

func newTestBlockListenerConf(t *testing.T, ctx context.Context, config *Config) (*blockListener, *rpcbackendmocks.WebSocketRPCClient) {

	logrus.SetLevel(logrus.DebugLevel)

	mRPC := rpcbackendmocks.NewWebSocketRPCClient(t)

	subsChan := make(chan *rpcbackend.RPCSubscriptionNotification)
	mSub := rpcbackendmocks.NewSubscription(t)
	mSub.On("Notifications").Return(subsChan).Maybe()

	mRPC.On("Connect", mock.Anything).Return(nil).Maybe()
	mRPC.On("Subscribe", mock.Anything, "newHeads").Return(
		mSub, nil,
	).Maybe()
	mRPC.On("UnsubscribeAll", mock.Anything).Return(nil).Maybe()
	mRPC.On("Close", mock.Anything).Return(nil).Maybe()

	bl, err := newBlockListener(ctx, config, &rpcclient.WSConfig{
		HTTPConfig: rpcclient.HTTPConfig{URL: "ws://localhost:0" /* unused per below re-wire to mRPC */}})
	assert.NoError(t, err)
	bl.wsConn = mRPC
	return bl, mRPC
}

func TestBlockListenerStartGettingHighestBlockRetry(t *testing.T) {

	ctx, bl, mRPC, done := newTestBlockListener(t)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").
		Return(&rpcbackend.RPCError{Message: "pop"}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(12345)
	})

	bl.start()

	h, err := bl.getHighestBlock(ctx)
	assert.Equal(t, uint64(12345), h)
	assert.NoError(t, err)
	done() // Stop immediately in this case, while we're in the polling interval

	<-bl.listenLoopDone

}

func TestBlockListenerStartGettingHighestBlockFailBeforeStop(t *testing.T) {

	_, bl, _, done := newTestBlockListener(t)
	done() // Stop before we start

	h, err := bl.getHighestBlock(context.Background())
	assert.Regexp(t, "PD010301", err)
	assert.Equal(t, uint64(0), h)

}

func TestBlockListenerStartGettingHighestBlockClosedCtx(t *testing.T) {

	_, bl, _, done := newTestBlockListener(t)
	defer done()

	closed, close := context.WithCancel(context.Background())
	close()
	h, err := bl.getHighestBlock(closed)
	assert.Regexp(t, "PD010301", err)
	assert.Equal(t, uint64(0), h)

}

func TestBlockListenerOKSequential(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond
	bl.unstableHeadLength = 2 // check wrapping

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
			block1002Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003Hash,
			ParentHash: block1002Hash,
		}
	})

	bl.start()

	assert.Equal(t, block1001Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1002Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1003Hash, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)

	assert.Equal(t, bl.unstableHeadLength, bl.canonicalChain.Len())

}

func TestBlockListenerWSShoulderTap(t *testing.T) {

	failedConnectOnce := false
	failedSubOnce := false
	toServer, fromServer, url, wsDone := wsclient.NewTestWSServer(func(req *http.Request) {
		if !failedConnectOnce {
			failedConnectOnce = true
			panic("fail once here")
		}
	})

	ctx, cancelCtx := context.WithCancel(context.Background())
	bl, err := newBlockListener(ctx, &Config{
		BlockPollingInterval: confutil.P("100s"), // so the test would just hang if no WS notifications
	}, &rpcclient.WSConfig{
		HTTPConfig: rpcclient.HTTPConfig{URL: url},
	})
	assert.NoError(t, err)
	defer cancelCtx()

	svrDone := make(chan struct{})
	pingerDone := make(chan struct{})
	complete := false
	go func() {
		defer close(svrDone)
		for {
			select {
			case rpcStr := <-toServer:
				var rpcReq rpcbackend.RPCRequest
				err := json.Unmarshal([]byte(rpcStr), &rpcReq)
				assert.NoError(t, err)
				rpcRes := &rpcbackend.RPCResponse{
					JSONRpc: rpcReq.JSONRpc,
					ID:      rpcReq.ID,
				}
				switch rpcReq.Method {
				case "eth_blockNumber":
					rpcRes.Result = fftypes.JSONAnyPtr(`"0x12345"`)
				case "eth_subscribe":
					assert.Equal(t, "newHeads", rpcReq.Params[0].AsString())
					if !failedSubOnce {
						failedSubOnce = true
						rpcRes.Error = &rpcbackend.RPCError{
							Code:    int64(rpcbackend.RPCCodeInternalError),
							Message: "pop",
						}
					} else {
						rpcRes.Result = fftypes.JSONAnyPtr(fmt.Sprintf(`"%s"`, uuid.New()))
						// Spam with notifications
						go func() {
							defer close(pingerDone)
							for !complete {
								time.Sleep(100 * time.Microsecond)
								if bl.newHeadsSub != nil {
									bl.newHeadsSub.Notifications() <- &rpcbackend.RPCSubscriptionNotification{
										CurrentSubID: bl.newHeadsSub.LocalID().String(),
										Result:       fftypes.JSONAnyPtr(`"anything"`),
									}
								}
							}
						}()
					}
				case "eth_newBlockFilter":
					rpcRes.Result = fftypes.JSONAnyPtr(fmt.Sprintf(`"%s"`, uuid.New()))
				case "eth_getFilterChanges":
					// ok we can close - the shoulder tap worked
					complete = true
					<-pingerDone
					go cancelCtx()
				default:
					assert.Fail(t, "unexpected RPC call: %+v", rpcReq)
				}
				b, err := json.Marshal(rpcRes)
				assert.NoError(t, err)
				fromServer <- string(b)
			case <-ctx.Done():
				return
			}
		}
	}()

	bl.start()

	// Wait until we close because it worked
	<-bl.listenLoopDone
	assert.True(t, failedConnectOnce)
	assert.True(t, failedSubOnce)

	bl.waitClosed()

	wsDone()
	<-svrDone
}

func TestBlockListenerOKDuplicates(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
			block1002Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1002Hash,
			block1003Hash,
		}
		go done() // once we've detected these duplicates, we can close
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003Hash,
			ParentHash: block1002Hash,
		}
	})

	bl.start()

	assert.Equal(t, block1001Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1002Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1003Hash, (<-bl.channel()).Hash)

	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)

}

func TestBlockListenerBlockNotAvailableAfterNotify(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1000Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		go done() // we're done
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1000Hash.String()
	}), false).Return(&rpcbackend.RPCError{Message: "not found"})

	bl.start()

	<-bl.listenLoopDone

	assert.Equal(t, uint64(1000), bl.highestBlock)

}

func TestBlockListenerReorgReplaceTail(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
			block1002Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashB,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1002Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashB.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashB,
			ParentHash: block1002Hash,
		}
	})

	bl.start()

	assert.Equal(t, block1001Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1002Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1003HashA, (<-bl.channel()).Hash)
	assert.Equal(t, block1003HashB, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)

}

func TestBlockListenerGap(t *testing.T) {

	// We have seen that certain JSON/RPC endpoints might miss blocks during re-orgs, and our listener
	// needs to cope with this. This means winding back when we find a gap and re-building our canonical
	// view of the chain.

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1004Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1005Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
			block1002HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1004Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002HashA.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1004Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1004),
			Hash:       block1004Hash,
			ParentHash: block1003Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1001
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashB,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003Hash,
			ParentHash: block1002HashB,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1004
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1004),
			Hash:       block1004Hash,
			ParentHash: block1003Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1005
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1005), // this one pops in while we're rebuilding
			Hash:       block1005Hash,
			ParentHash: block1004Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1006 // not found
	}), false).Return(nil)

	bl.start()

	assert.Equal(t, block1001Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1002HashA, (<-bl.channel()).Hash)
	assert.Equal(t, block1002HashB, (<-bl.channel()).Hash)
	assert.Equal(t, block1003Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1004Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1005Hash, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1005), bl.highestBlock)

}

func TestBlockListenerRebuildToHead(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1001
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), false).Return(&rpcbackend.RPCError{Message: "not found"})

	bl.start()

	assert.Equal(t, block1001Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1002HashA, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)

}

func TestBlockListenerReorgWhileRebuilding(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1001
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashB, // this is a re-org'd block, so we stop here as if we've found the end of the chain
			ParentHash: block1002HashB,
		}
	})

	bl.start()

	assert.Equal(t, block1001Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1002HashA, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)

}

func TestBlockListenerReorgReplaceWholeCanonicalChain(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1002HashA,
			block1003HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashB,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002HashA.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1002HashA,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashB.String()
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashB,
			ParentHash: block1002HashB,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashB,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), false).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashB,
			ParentHash: block1002HashB,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1004 // not found
	}), false).Return(nil)

	bl.start()

	assert.Equal(t, block1002HashA, (<-bl.channel()).Hash)
	assert.Equal(t, block1003HashA, (<-bl.channel()).Hash)
	assert.Equal(t, block1002HashB, (<-bl.channel()).Hash)
	assert.Equal(t, block1003HashB, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)

}

func TestBlockListenerBlockNotFound(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		go done() // Close after we've processed the log
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), false).Return(nil)

	bl.start()

	bl.waitClosed()

}

func TestBlockListenerBlockHashFailed(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		go done() // Close after we've processed the log
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), false).Return(&rpcbackend.RPCError{Message: "pop"})

	bl.start()

	bl.waitClosed()

}

func TestBlockListenerReestablishBlockFilter(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id2"
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(&rpcbackend.RPCError{Message: "filter not found"}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		go done() // Close after we've processed the log
	})

	bl.start()
	bl.waitClosed()

}

func TestBlockListenerReestablishBlockFilterFail(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(&rpcbackend.RPCError{Message: "pop"}).Run(func(args mock.Arguments) {
		go done()
	})

	bl.start()
	bl.waitClosed()

}

func TestBlockListenerDispatchStopped(t *testing.T) {
	_, bl, _, done := newTestBlockListener(t)
	done()
	bl.newBlocks = make(chan *BlockInfoJSONRPC)
	// Will not block when context is cancelled
	bl.notifyBlock(&BlockInfoJSONRPC{})
}

func TestBlockListenerRebuildCanonicalChainEmpty(t *testing.T) {

	_, bl, _, done := newTestBlockListener(t)
	done()

	res := bl.rebuildCanonicalChain()
	assert.Nil(t, res)

}

func TestBlockListenerRebuildCanonicalFailTerminate(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	defer done()
	bl.canonicalChain.PushBack(&BlockInfoJSONRPC{
		Number:     1000,
		Hash:       ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32)),
		ParentHash: ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32)),
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.Anything, false).
		Return(&rpcbackend.RPCError{Message: "pop"}).
		Run(func(args mock.Arguments) {
			done()
		})

	res := bl.rebuildCanonicalChain()
	assert.Nil(t, res)

}

func TestBlockListenerClosedBeforeEstablishingBlockHeight(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	done()

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").
		Return(&rpcbackend.RPCError{Message: "pop"}).Once()

	bl.listenLoopDone = make(chan struct{})
	bl.listenLoop()

}
