---
title: bidx_*
---
## `bidx_decodeTransactionEvents`

### Parameters

0. `transactionHash`: [`Bytes32`](../types/simpletypes.md#bytes32)
1. `abi`: [`Entry[]`](../types/transactioninput.md#entry)
2. `resultFormat`: [`JSONFormatOptions`](../types/jsonformatoptions.md#jsonformatoptions)

### Returns

0. `events`: [`EventWithData[]`](../types/eventwithdata.md#eventwithdata)

## `bidx_getBlockByNumber`

### Parameters

0. `blockNumber`: [`HexUint64`](../types/simpletypes.md#hexuint64)

### Returns

0. `block`: [`IndexedBlock`](../types/indexedblock.md#indexedblock)

## `bidx_getBlockTransactionsByNumber`

### Parameters

0. `blockNumber`: [`HexUint64`](../types/simpletypes.md#hexuint64)

### Returns

0. `transactions`: [`IndexedTransaction[]`](../types/indexedtransaction.md#indexedtransaction)

## `bidx_getConfirmedBlockHeight`

### Returns

0. `blockHeight`: [`HexUint64`](../types/simpletypes.md#hexuint64)

## `bidx_getTransactionByHash`

### Parameters

0. `blockHash`: [`Bytes32`](../types/simpletypes.md#bytes32)

### Returns

0. `transaction`: [`IndexedTransaction`](../types/indexedtransaction.md#indexedtransaction)

## `bidx_getTransactionByNonce`

### Parameters

0. `from`: [`EthAddress`](../types/simpletypes.md#ethaddress)
1. `nonce`: [`HexUint64`](../types/simpletypes.md#hexuint64)

### Returns

0. `transaction`: [`IndexedTransaction`](../types/indexedtransaction.md#indexedtransaction)

## `bidx_getTransactionEventsByHash`

### Parameters

0. `transactionHash`: [`Bytes32`](../types/simpletypes.md#bytes32)

### Returns

0. `events`: [`IndexedEvent[]`](../types/indexedevent.md#indexedevent)

## `bidx_queryIndexedBlocks`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

### Returns

0. `blocks`: [`IndexedBlock[]`](../types/indexedblock.md#indexedblock)

## `bidx_queryIndexedEvents`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

### Returns

0. `events`: [`IndexedEvent[]`](../types/indexedevent.md#indexedevent)

## `bidx_queryIndexedTransactions`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

### Returns

0. `transactions`: [`IndexedTransaction[]`](../types/indexedtransaction.md#indexedtransaction)

