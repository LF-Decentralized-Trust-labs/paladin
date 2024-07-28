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
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/mocks/rpcbackendmocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestBlockListener(t *testing.T) (context.Context, *blockListener, *rpcbackendmocks.WebSocketRPCClient, func()) {
	mRPC := rpcbackendmocks.NewWebSocketRPCClient(t)

	subsChan := make(chan *rpcbackend.RPCSubscriptionNotification)
	mSub := rpcbackendmocks.NewSubscription(t)
	mSub.On("Notifications").Return(subsChan)

	mRPC.On("Connect", mock.Anything).Return(nil)
	mRPC.On("Subscribe", mock.Anything, "newHeads").Return(
		mSub, nil,
	)

	ctx, cancelCtx := context.WithCancel(context.Background())
	bl, err := newBlockListener(ctx, &Config{}, &RPCWSConnectConfig{})
	assert.NoError(t, err)
	bl.wsConn = mRPC
	return ctx, bl, mRPC, cancelCtx
}

func TestBlockListenerStartGettingHighestBlockRetry(t *testing.T) {

	ctx, bl, mRPC, done := newTestBlockListener(t)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").
		Return(&rpcbackend.RPCError{Message: "pop"}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(12345)
	})

	h, ok := bl.getHighestBlock(ctx)
	assert.Equal(t, uint64(12345), h)
	assert.True(t, ok)
	done() // Stop immediately in this case, while we're in the polling interval

	<-bl.listenLoopDone

	mRPC.AssertExpectations(t)

}

func TestBlockListenerStartGettingHighestBlockFailBeforeStop(t *testing.T) {

	ctx, bl, mRPC, done := newTestBlockListener(t)
	done() // Stop before we start

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").
		Return(&rpcbackend.RPCError{Message: "pop"}).Maybe()

	h, ok := bl.getHighestBlock(ctx)
	assert.False(t, ok)
	assert.Equal(t, uint64(0), h)

	<-bl.listenLoopDone

	mRPC.AssertExpectations(t)

}

// func TestBlockListenerOKSequential(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond
// 	bl.unstableHeadLength = 2 // check wrapping

// 	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1001Hash,
// 			block1002Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1001Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1001),
// 			Hash:       block1001Hash,
// 			ParentHash: block1000Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1002Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1002),
// 			Hash:       block1002Hash,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003Hash,
// 			ParentHash: block1002Hash,
// 		}
// 	})

// 	updates := make(chan *ffcapi.BlockHashEvent)
// 	bl.addConsumer(&blockUpdateConsumer{
// 		id:      fftypes.NewUUID(),
// 		ctx:     context.Background(),
// 		updates: updates,
// 	})

// 	bu := <-updates
// 	assert.Equal(t, []string{
// 		block1001Hash.String(),
// 		block1002Hash.String(),
// 	}, bu.BlockHashes)
// 	bu = <-updates
// 	assert.Equal(t, []string{
// 		block1003Hash.String(),
// 	}, bu.BlockHashes)
// 	assert.False(t, bu.GapPotential)

// 	done()
// 	<-bl.listenLoopDone

// 	assert.Equal(t, int64(1003), bl.highestBlock)

// 	mRPC.AssertExpectations(t)

// 	assert.Equal(t, bl.unstableHeadLength, bl.canonicalChain.Len())

// }

// func TestBlockListenerWSShoulderTap(t *testing.T) {

// 	failedConnectOnce := false
// 	failedSubOnce := false
// 	toServer, fromServer, url, wsDone := wsclient.NewTestWSServer(func(req *http.Request) {
// 		if !failedConnectOnce {
// 			failedConnectOnce = true
// 			panic("fail once here")
// 		}
// 	})

// 	ctx, c, _, done := newTestConnector(t, func(conf config.Section) {
// 		conf.Set(wsclient.WSConfigURL, url)
// 		conf.Set(wsclient.WSConfigKeyInitialConnectAttempts, 0)
// 		conf.Set(WebSocketsEnabled, true)
// 		conf.Set(BlockPollingInterval, "100s") // so the test would just hang if no WS notifications
// 	})
// 	svrDone := make(chan struct{})

// 	pingerDone := make(chan struct{})
// 	complete := false
// 	go func() {
// 		defer close(svrDone)
// 		for {
// 			select {
// 			case rpcStr := <-toServer:
// 				var rpcReq rpcbackend.RPCRequest
// 				err := json.Unmarshal([]byte(rpcStr), &rpcReq)
// 				assert.NoError(t, err)
// 				rpcRes := &rpcbackend.RPCResponse{
// 					JSONRpc: rpcReq.JSONRpc,
// 					ID:      rpcReq.ID,
// 				}
// 				switch rpcReq.Method {
// 				case "eth_blockNumber":
// 					rpcRes.Result = fftypes.JSONAnyPtr(`"0x12345"`)
// 				case "eth_subscribe":
// 					assert.Equal(t, "newHeads", rpcReq.Params[0].AsString())
// 					if !failedSubOnce {
// 						failedSubOnce = true
// 						rpcRes.Error = &rpcbackend.RPCError{
// 							Code:    int64(rpcbackend.RPCCodeInternalError),
// 							Message: "pop",
// 						}
// 					} else {
// 						rpcRes.Result = fftypes.JSONAnyPtr(fmt.Sprintf(`"%s"`, fftypes.NewUUID()))
// 						// Spam with notifications
// 						go func() {
// 							defer close(pingerDone)
// 							for !complete {
// 								time.Sleep(100 * time.Microsecond)
// 								if bl.newHeadsSub != nil {
// 									bl.newHeadsSub.Notifications() <- &rpcbackend.RPCSubscriptionNotification{
// 										CurrentSubID: bl.newHeadsSub.LocalID().String(),
// 										Result:       fftypes.JSONAnyPtr(`"anything"`),
// 									}
// 								}
// 							}
// 						}()
// 					}
// 				case "eth_newBlockFilter":
// 					rpcRes.Result = fftypes.JSONAnyPtr(fmt.Sprintf(`"%s"`, fftypes.NewUUID()))
// 				case "eth_getFilterChanges":
// 					// ok we can close - the shoulder tap worked
// 					complete = true
// 					<-pingerDone
// 					go done()
// 				default:
// 					assert.Fail(t, "unexpected RPC call: %+v", rpcReq)
// 				}
// 				b, err := json.Marshal(rpcRes)
// 				assert.NoError(t, err)
// 				fromServer <- string(b)
// 			case <-ctx.Done():
// 				return
// 			}
// 		}
// 	}()

// 	bl.checkStartedLocked()

// 	// Wait until we close because it worked
// 	<-bl.listenLoopDone
// 	assert.True(t, failedConnectOnce)
// 	assert.True(t, failedSubOnce)

// 	wsDone()
// 	<-svrDone
// }

// func TestBlockListenerOKDuplicates(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond

// 	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1001Hash,
// 			block1002Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1002Hash,
// 			block1003Hash,
// 		}
// 		go done() // once we've detected these duplicates, we can close
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1001Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1001),
// 			Hash:       block1001Hash,
// 			ParentHash: block1000Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1002Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1002),
// 			Hash:       block1002Hash,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003Hash,
// 			ParentHash: block1002Hash,
// 		}
// 	})

// 	updates := make(chan *ffcapi.BlockHashEvent)
// 	bl.addConsumer(&blockUpdateConsumer{
// 		id:      fftypes.NewUUID(),
// 		ctx:     context.Background(),
// 		updates: updates,
// 	})

// 	bu := <-updates
// 	assert.Equal(t, []string{
// 		block1001Hash.String(),
// 		block1002Hash.String(),
// 	}, bu.BlockHashes)
// 	bu = <-updates
// 	assert.Equal(t, []string{
// 		block1003Hash.String(),
// 	}, bu.BlockHashes)
// 	assert.False(t, bu.GapPotential)

// 	<-bl.listenLoopDone

// 	assert.Equal(t, int64(1003), bl.highestBlock)

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerReorgReplaceTail(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond

// 	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1001Hash,
// 			block1002Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003HashA,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003HashB,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1001Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1001),
// 			Hash:       block1001Hash,
// 			ParentHash: block1000Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1002Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1002),
// 			Hash:       block1002Hash,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003HashA.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003HashA,
// 			ParentHash: block1002Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003HashB.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003HashB,
// 			ParentHash: block1002Hash,
// 		}
// 	})

// 	updates := make(chan *ffcapi.BlockHashEvent)
// 	bl.addConsumer(&blockUpdateConsumer{
// 		id:      fftypes.NewUUID(),
// 		ctx:     context.Background(),
// 		updates: updates,
// 	})

// 	bu := <-updates
// 	assert.Equal(t, []string{
// 		block1001Hash.String(),
// 		block1002Hash.String(),
// 	}, bu.BlockHashes)
// 	bu = <-updates
// 	assert.Equal(t, []string{
// 		block1003HashA.String(),
// 	}, bu.BlockHashes)
// 	assert.False(t, bu.GapPotential)
// 	bu = <-updates
// 	assert.Equal(t, []string{
// 		block1003HashB.String(),
// 	}, bu.BlockHashes)
// 	assert.False(t, bu.GapPotential)

// 	done()
// 	<-bl.listenLoopDone

// 	assert.Equal(t, int64(1003), bl.highestBlock)

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerGap(t *testing.T) {

// 	// See issue https://github.com/hyperledger/firefly-evmconnect/issues/10
// 	// We have seen that certain JSON/RPC endpoints might miss blocks during re-orgs, and our listener
// 	// needs to cope with this. This means winding back when we find a gap and re-building our canonical
// 	// view of the chain.

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond

// 	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1004Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1005Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1001Hash,
// 			block1002HashA,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1004Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1001Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1001),
// 			Hash:       block1001Hash,
// 			ParentHash: block1000Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1002HashA.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1002),
// 			Hash:       block1002HashA,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1004Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1004),
// 			Hash:       block1004Hash,
// 			ParentHash: block1003Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1001
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1001),
// 			Hash:       block1001Hash,
// 			ParentHash: block1000Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1002
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1002),
// 			Hash:       block1002HashB,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1003
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003Hash,
// 			ParentHash: block1002HashB,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1004
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1004),
// 			Hash:       block1004Hash,
// 			ParentHash: block1003Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1005
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1005), // this one pops in while we're rebuilding
// 			Hash:       block1005Hash,
// 			ParentHash: block1004Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1006 // not found
// 	}), false).Return(nil)

// 	updates := make(chan *ffcapi.BlockHashEvent)
// 	bl.addConsumer(&blockUpdateConsumer{
// 		id:      fftypes.NewUUID(),
// 		ctx:     context.Background(),
// 		updates: updates,
// 	})

// 	bu := <-updates
// 	assert.Equal(t, []string{
// 		block1001Hash.String(),
// 		block1002HashA.String(),
// 	}, bu.BlockHashes)
// 	bu = <-updates
// 	assert.Equal(t, []string{
// 		block1002HashB.String(),
// 		block1003Hash.String(), // The gap we filled in
// 		block1004Hash.String(),
// 		block1005Hash.String(), // Appeared while we were rebuilding our chain
// 	}, bu.BlockHashes)
// 	assert.False(t, bu.GapPotential)

// 	done()
// 	<-bl.listenLoopDone

// 	assert.Equal(t, int64(1005), bl.highestBlock)

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerReorgWhileRebuilding(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond

// 	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1001Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003HashA,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1001Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1001),
// 			Hash:       block1001Hash,
// 			ParentHash: block1000Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003HashA.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003HashA,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1001
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1001),
// 			Hash:       block1001Hash,
// 			ParentHash: block1000Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1002
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1002),
// 			Hash:       block1002HashA,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1003
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003HashB, // this is a re-org'd block, so we stop here as if we've found the end of the chain
// 			ParentHash: block1002HashB,
// 		}
// 	})

// 	updates := make(chan *ffcapi.BlockHashEvent)
// 	bl.addConsumer(&blockUpdateConsumer{
// 		id:      fftypes.NewUUID(),
// 		ctx:     context.Background(),
// 		updates: updates,
// 	})

// 	bu := <-updates
// 	assert.Equal(t, []string{
// 		block1001Hash.String(),
// 	}, bu.BlockHashes)
// 	bu = <-updates
// 	assert.Equal(t, []string{
// 		block1002HashA.String(),
// 	}, bu.BlockHashes)

// 	done()
// 	<-bl.listenLoopDone

// 	assert.Equal(t, int64(1003), bl.highestBlock)

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerReorgReplaceWholeCanonicalChain(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond

// 	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1002HashA,
// 			block1003HashA,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003HashB,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1002HashA.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1002),
// 			Hash:       block1002HashA,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003HashA.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003HashA,
// 			ParentHash: block1002HashA,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003HashB.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003HashB,
// 			ParentHash: block1002HashB,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1002
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1002),
// 			Hash:       block1002HashB,
// 			ParentHash: block1001Hash,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1003
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003HashB,
// 			ParentHash: block1002HashB,
// 		}
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn *ethtypes.HexInteger) bool {
// 		return bn.BigInt().Int64() == 1004 // not found
// 	}), false).Return(nil)

// 	updates := make(chan *ffcapi.BlockHashEvent)
// 	bl.addConsumer(&blockUpdateConsumer{
// 		id:      fftypes.NewUUID(),
// 		ctx:     context.Background(),
// 		updates: updates,
// 	})

// 	bu := <-updates
// 	assert.Equal(t, []string{
// 		block1002HashA.String(),
// 		block1003HashA.String(),
// 	}, bu.BlockHashes)
// 	bu = <-updates
// 	assert.Equal(t, []string{
// 		block1002HashB.String(),
// 		block1003HashB.String(),
// 	}, bu.BlockHashes)
// 	assert.False(t, bu.GapPotential)

// 	done()
// 	<-bl.listenLoopDone

// 	assert.Equal(t, int64(1003), bl.highestBlock)

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerClosed(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond
// 	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
// 		if len(bl.consumers) == 0 {
// 			go done() // Close after we've processed the log
// 		}
// 	})

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003Hash.String()
// 	}), false).Return(nil).Run(func(args mock.Arguments) {
// 		*args[1].(**blockInfoJSONRPC) = &blockInfoJSONRPC{
// 			Number:     ethtypes.NewHexInteger64(1003),
// 			Hash:       block1003Hash,
// 			ParentHash: block1002Hash,
// 		}
// 	})

// 	updates := make(chan *ffcapi.BlockHashEvent)
// 	cancelledCtx, cCancel := context.WithCancel(context.Background())
// 	cCancel()
// 	bl.addConsumer(&blockUpdateConsumer{
// 		id:      fftypes.NewUUID(),
// 		ctx:     cancelledCtx,
// 		updates: updates,
// 	})

// 	c.WaitClosed()
// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerBlockNotFound(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond
// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
// 		go done() // Close after we've processed the log
// 	})

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003Hash.String()
// 	}), false).Return(nil)

// 	bl.checkStartedLocked()

// 	c.WaitClosed()

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerBlockHashFailed(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond
// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
// 		go done() // Close after we've processed the log
// 	})

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == block1003Hash.String()
// 	}), false).Return(&rpcbackend.RPCError{Message: "pop"})

// 	bl.checkStartedLocked()

// 	c.WaitClosed()

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerProcessNonStandardHashRejectedWhenNotInHederaCompatibilityMode(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond
// 	bl.hederaCompatibilityMode = false

// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix("0xef177df3b87beed681b1557e8ba7c3ecbd7e4db83d87b66c1e86aa484937ab93f1fae0eb6d4b24ca30aee13f29c83cc9")

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
// 		go done() // Close after we've processed the log
// 	})

// 	bl.checkStartedLocked()

// 	c.WaitClosed()

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerProcessNonStandardHashRejectedWhenWrongSizeForHedera(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond
// 	bl.hederaCompatibilityMode = true

// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix("0xef")

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
// 		go done() // Close after we've processed the log
// 	})

// 	bl.checkStartedLocked()

// 	c.WaitClosed()

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerProcessNonStandardHashAcceptedWhenInHederaCompatbilityMode(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond
// 	bl.hederaCompatibilityMode = true

// 	block1003Hash := ethtypes.MustNewHexBytes0xPrefix("0xef177df3b87beed681b1557e8ba7c3ecbd7e4db83d87b66c1e86aa484937ab93f1fae0eb6d4b24ca30aee13f29c83cc9")
// 	truncatedBlock1003Hash := ethtypes.MustNewHexBytes0xPrefix("0xef177df3b87beed681b1557e8ba7c3ecbd7e4db83d87b66c1e86aa484937ab93")

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	})
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
// 		*hbh = []ethtypes.HexBytes0xPrefix{
// 			block1003Hash,
// 		}
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
// 		go done() // Close after we've processed the log
// 	})

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
// 		return bh == truncatedBlock1003Hash.String()
// 	}), false).Return(&rpcbackend.RPCError{Message: "pop"})

// 	bl.checkStartedLocked()

// 	c.WaitClosed()

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerReestablishBlockFilter(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id1"
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*string)
// 		*hbh = "filter_id2"
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(&rpcbackend.RPCError{Message: "filter not found"}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
// 		go done() // Close after we've processed the log
// 	})

// 	bl.checkStartedLocked()

// 	c.WaitClosed()

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerReestablishBlockFilterFail(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.blockPollingInterval = 1 * time.Microsecond

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
// 		hbh := args[1].(*ethtypes.HexInteger)
// 		*hbh = *ethtypes.NewHexInteger64(1000)
// 	}).Once()
// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(&rpcbackend.RPCError{Message: "pop"}).Run(func(args mock.Arguments) {
// 		go done()
// 	})

// 	bl.checkStartedLocked()

// 	c.WaitClosed()

// 	mRPC.AssertExpectations(t)

// }

// func TestBlockListenerDispatchStopped(t *testing.T) {
// 	_, c, _, done := newTestConnector(t)
// 	done()

// 	c.blockListener.dispatchToConsumers([]*blockUpdateConsumer{
// 		{id: fftypes.NewUUID(), ctx: context.Background(), updates: make(chan<- *ffcapi.BlockHashEvent)},
// 	}, &ffcapi.BlockHashEvent{
// 		BlockHashes: []string{},
// 	})
// }

// func TestBlockListenerRebuildCanonicalChainEmpty(t *testing.T) {

// 	_, c, _, done := newTestConnector(t)
// 	defer done()

// 	res := bl.rebuildCanonicalChain()
// 	assert.Nil(t, res)

// }

// func TestBlockListenerRebuildCanonicalFailTerminate(t *testing.T) {

// 	ctx, bl, mRPC, done := newTestBlockListener(t)
// 	bl.canonicalChain.PushBack(&minimalBlockInfo{
// 		number:     1000,
// 		hash:       ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String()).String(),
// 		parentHash: ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String()).String(),
// 	})

// 	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.Anything, false).
// 		Return(&rpcbackend.RPCError{Message: "pop"}).
// 		Run(func(args mock.Arguments) {
// 			done()
// 		})

// 	res := bl.rebuildCanonicalChain()
// 	assert.Nil(t, res)

// 	mRPC.AssertExpectations(t)
// }
