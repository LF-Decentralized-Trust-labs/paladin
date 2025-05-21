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
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/mocks/rpcclientmocks"
	"github.com/kaleido-io/paladin/sdk/go/pkg/wsclient"

	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testBlockFilterID1 = "block_filter_1"
const testBlockFilterID2 = "block_filter_2"

func newTestBlockListener(t *testing.T) (context.Context, *blockListener, *rpcclientmocks.WSClient, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	bl, mRPC := newTestBlockListenerConf(t, ctx, &pldconf.BlockIndexerConfig{})
	return ctx, bl, mRPC, func() {
		cancelCtx()
		bl.waitClosed()
	}
}

func newTestBlockListenerConf(t *testing.T, ctx context.Context, config *pldconf.BlockIndexerConfig) (*blockListener, *rpcclientmocks.WSClient) {

	mRPC := rpcclientmocks.NewWSClient(t)

	subsChan := make(chan rpcclient.RPCSubscriptionNotification)
	mSub := rpcclientmocks.NewSubscription(t)
	mSub.On("Notifications").Return(subsChan).Maybe()

	mRPC.On("Connect", mock.Anything).Return(nil).Maybe()
	mRPC.On("Subscribe", mock.Anything, mock.Anything, "newHeads").Return(
		mSub, nil,
	).Maybe()
	mRPC.On("UnsubscribeAll", mock.Anything).Return(nil).Maybe()
	mRPC.On("Close", mock.Anything).Return(nil).Maybe()

	bl, err := newBlockListener(ctx, config, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: "ws://localhost:0" /* unused per below re-wire to mRPC */}})
	require.NoError(t, err)
	bl.wsConn = mRPC
	return bl, mRPC
}

func TestBlockListenerStartGettingHighestBlockRetry(t *testing.T) {
	ctx, bl, mRPC, done := newTestBlockListener(t)

	// Mock eth_newBlockFilter call (happens on initialization)
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").
		Return(nil).Once()

	// Mock eth_getFilterChanges call (this is called after eth_newBlockFilter)
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "").
		Return(nil).Once()

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").
		Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("pop"))).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(12345)
	})

	bl.start()

	h, err := bl.getHighestBlock(ctx)
	assert.Equal(t, uint64(12345), h)
	require.NoError(t, err)
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

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
			block1002Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
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
	bl, err := newBlockListener(ctx, &pldconf.BlockIndexerConfig{
		BlockPollingInterval: confutil.P("100s"), // so the test would just hang if no WS notifications
	}, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	})
	require.NoError(t, err)
	defer cancelCtx()

	svrDone := make(chan struct{})
	pingerDone := make(chan struct{})
	complete := false
	go func() {
		defer close(svrDone)
		for {
			select {
			case rpcStr := <-toServer:
				var rpcReq rpcclient.RPCRequest
				err := json.Unmarshal([]byte(rpcStr), &rpcReq)
				require.NoError(t, err)
				rpcRes := &rpcclient.RPCResponse{
					JSONRpc: rpcReq.JSONRpc,
					ID:      rpcReq.ID,
				}
				switch rpcReq.Method {
				case "eth_blockNumber":
					rpcRes.Result = pldtypes.RawJSON(`"0x12345"`)
				case "eth_subscribe":
					assert.Equal(t, "newHeads", rpcReq.Params[0].StringValue())
					if !failedSubOnce {
						failedSubOnce = true
						rpcRes.Error = &rpcclient.RPCError{
							Code:    int64(rpcclient.RPCCodeInternalError),
							Message: "pop",
						}
					} else {
						rpcRes.Result = pldtypes.RawJSON(fmt.Sprintf(`"%s"`, uuid.New()))
						// Spam with notifications
						go func() {
							defer close(pingerDone)
							for !complete {
								time.Sleep(100 * time.Microsecond)
								if bl.newHeadsSub != nil {
									bl.newHeadsSub.Notifications() <- rpcclientmocks.NewRPCSubscriptionNotification(t)
								}
							}
						}()
					}
				case "eth_newBlockFilter":
					rpcRes.Result = pldtypes.RawJSON(fmt.Sprintf(`"%s"`, uuid.New()))
				case "eth_getFilterChanges":
					// ok we can close - the shoulder tap worked
					complete = true
					<-pingerDone
					go cancelCtx()
				default:
					assert.Fail(t, "unexpected RPC call: %+v", rpcReq)
				}
				b, err := json.Marshal(rpcRes)
				require.NoError(t, err)
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

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
			block1002Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
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
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
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

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1000Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		go done() // we're done
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1000Hash.String()
	}), true).Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("not found")))

	bl.start()

	<-bl.listenLoopDone

	assert.Equal(t, uint64(1000), bl.highestBlock)

}

func TestBlockListenerReorgKeepLatestHeadInSameBatch(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String()) // parent
	block1001HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1001HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(
		func(args mock.Arguments) {
			hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
			*hbh = []ethtypes.HexBytes0xPrefix{
				block1001HashA,
				block1001HashB,
				block1002Hash,
				block1003Hash,
			}
		})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001HashA,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001HashB.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001HashB,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001HashB,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003Hash,
			ParentHash: block1002Hash,
		}
	})

	bl.start()

	assert.Equal(t, block1001HashB, (<-bl.channel()).Hash)
	assert.Equal(t, block1002Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1003Hash, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)
}

func TestBlockListenerReorgKeepLatestHeadInSameBatchValidHashFirst(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String()) // parent
	block1001HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1001HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(
		func(args mock.Arguments) {
			hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
			*hbh = []ethtypes.HexBytes0xPrefix{
				block1001HashB, // valid hash is in the front of the array, so will need to re-build the chain
				block1001HashA,
				block1002Hash,
				block1003Hash,
			}
		})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1001
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001HashB,
			ParentHash: block1000Hash,
		}
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001HashB,
		}
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003Hash,
			ParentHash: block1002Hash,
		}
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1004 // not found
	}), true).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001HashA,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001HashB.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001HashB,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001HashB,
		}
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003Hash,
			ParentHash: block1002Hash,
		}
	})
	bl.start()

	assert.Equal(t, block1001HashB, (<-bl.channel()).Hash)
	assert.Equal(t, block1002Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1003Hash, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)
}

func TestBlockListenerReorgKeepLatestMiddleInSameBatch(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String()) // parent
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(
		func(args mock.Arguments) {
			hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
			*hbh = []ethtypes.HexBytes0xPrefix{
				block1001Hash,
				block1002HashA,
				block1002HashB,
				block1003Hash,
			}
		})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002HashB.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashB,
			ParentHash: block1001Hash,
		}
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003Hash,
			ParentHash: block1002HashB,
		}
	})
	bl.start()

	assert.Equal(t, block1001Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1002HashB, (<-bl.channel()).Hash)
	assert.Equal(t, block1003Hash, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)
}

func TestBlockListenerReorgKeepLatestTailInSameBatch(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String()) // parent
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(fftypes.NewRandB32().String())

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(
		func(args mock.Arguments) {
			hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
			*hbh = []ethtypes.HexBytes0xPrefix{
				block1001Hash,
				block1002Hash,
				block1003HashA,
				block1003HashB,
			}
		})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1002Hash,
		}
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashB.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashB,
			ParentHash: block1002Hash,
		}
	})

	bl.start()

	assert.Equal(t, block1001Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1002Hash, (<-bl.channel()).Hash)
	assert.Equal(t, block1003HashB, (<-bl.channel()).Hash)

	done()
	<-bl.listenLoopDone

	assert.Equal(t, uint64(1003), bl.highestBlock)
}

func TestBlockListenerReorgReplaceTail(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
			block1002Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashB,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002Hash,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1002Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashB.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
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

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1004Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1005Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
			block1002HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1004Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1004Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1004),
			Hash:       block1004Hash,
			ParentHash: block1003Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1001
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashB,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003Hash,
			ParentHash: block1002HashB,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1004
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1004),
			Hash:       block1004Hash,
			ParentHash: block1003Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1005
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1005), // this one pops in while we're rebuilding
			Hash:       block1005Hash,
			ParentHash: block1004Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1006 // not found
	}), true).Return(nil)

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

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1001
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), true).Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("not found")))

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

	block1000Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1001Hash,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1001Hash.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1001
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1001),
			Hash:       block1001Hash,
			ParentHash: block1000Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), true).Return(nil).Run(func(args mock.Arguments) {
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

	block1001Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002HashA := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003HashA := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1002HashB := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	block1003HashB := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1002HashA,
			block1003HashA,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{
			block1003HashB,
		}
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", mock.Anything).Return(nil)

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1002HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashA,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashA.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashA,
			ParentHash: block1002HashA,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByHash", mock.MatchedBy(func(bh string) bool {
		return bh == block1003HashB.String()
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashB,
			ParentHash: block1002HashB,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1002
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1002),
			Hash:       block1002HashB,
			ParentHash: block1001Hash,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1003
	}), true).Return(nil).Run(func(args mock.Arguments) {
		*args[1].(**BlockInfoJSONRPC) = &BlockInfoJSONRPC{
			Number:     ethtypes.HexUint64(1003),
			Hash:       block1003HashB,
			ParentHash: block1002HashB,
		}
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.MatchedBy(func(bn ethtypes.HexUint64) bool {
		return bn == 1004 // not found
	}), true).Return(nil)

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
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
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
	}), true).Return(nil)

	bl.start()

	bl.waitClosed()

}

func TestBlockListenerBlockHashFailed(t *testing.T) {

	_, bl, mRPC, done := newTestBlockListener(t)
	bl.blockPollingInterval = 1 * time.Microsecond
	block1003Hash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(1000)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID1
	})
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(nil).Run(func(args mock.Arguments) {
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
	}), true).Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("pop")))

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
		*hbh = testBlockFilterID1
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = testBlockFilterID2
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", testBlockFilterID1).Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("filter not found"))).Once()
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
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("pop"))).Run(func(args mock.Arguments) {
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
		Hash:       ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32)),
		ParentHash: ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32)),
	})

	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.Anything, true).
		Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("pop"))).
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
		Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("pop"))).Once()

	bl.listenLoopDone = make(chan struct{})
	listenerInitiated := make(chan struct{})
	bl.listenLoop(&listenerInitiated)

}
