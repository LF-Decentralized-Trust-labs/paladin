---
title: ptx_*
---
## `ptx_call`

### Parameters

0. `transaction`: [`TransactionCall`](../types/transactioncall.md#transactioncall)

### Returns

0. `result`: [`RawJSON`](../types/simpletypes.md#rawjson)

## `ptx_decodeCall`

### Parameters

0. `callData`: [`HexBytes`](../types/simpletypes.md#hexbytes)
1. `dataFormat`: [`JSONFormatOptions`](../types/jsonformatoptions.md#jsonformatoptions)

### Returns

0. `decodedCall`: [`ABIDecodedData`](../types/abidecodeddata.md#abidecodeddata)

## `ptx_decodeError`

### Parameters

0. `revertData`: [`HexBytes`](../types/simpletypes.md#hexbytes)
1. `dataFormat`: [`JSONFormatOptions`](../types/jsonformatoptions.md#jsonformatoptions)

### Returns

0. `decodedError`: [`ABIDecodedData`](../types/abidecodeddata.md#abidecodeddata)

## `ptx_decodeEvent`

### Parameters

0. `topics`: [`Bytes32[]`](../types/simpletypes.md#bytes32)
1. `data`: [`HexBytes`](../types/simpletypes.md#hexbytes)
2. `dataFormat`: [`JSONFormatOptions`](../types/jsonformatoptions.md#jsonformatoptions)

### Returns

0. `decodedEvent`: [`ABIDecodedData`](../types/abidecodeddata.md#abidecodeddata)

## `ptx_getDomainReceipt`

### Parameters

0. `domain`: `string`
1. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

### Returns

0. `domainReceipt`: [`RawJSON`](../types/simpletypes.md#rawjson)

## `ptx_getPreparedTransaction`

### Parameters

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

### Returns

0. `preparedTransaction`: [`PreparedTransaction`](../types/preparedtransaction.md#preparedtransaction)

## `ptx_getStateReceipt`

### Parameters

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

### Returns

0. `stateReceipt`: [`TransactionStates`](../types/transactionstates.md#transactionstates)

## `ptx_getStoredABI`

### Parameters

0. `hashRef`: [`Bytes32`](../types/simpletypes.md#bytes32)

### Returns

0. `storedABI`: [`StoredABI`](../types/storedabi.md#storedabi)

## `ptx_getTransaction`

### Parameters

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

### Returns

0. `transaction`: [`Transaction`](../types/transaction.md#transaction)

## `ptx_getTransactionByIdempotencyKey`

### Parameters

0. `idempotencyKey`: `string`

### Returns

0. `transaction`: [`Transaction`](../types/transaction.md#transaction)

## `ptx_getTransactionFull`

### Parameters

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

### Returns

0. `transaction`: [`TransactionFull`](../types/transactionfull.md#transactionfull)

## `ptx_getTransactionReceipt`

### Parameters

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

### Returns

0. `receipt`: [`TransactionReceipt`](../types/transactionreceipt.md#transactionreceipt)

## `ptx_getTransactionReceiptFull`

### Parameters

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

### Returns

0. `receipt`: [`TransactionReceiptFull`](../types/transactionreceiptfull.md#transactionreceiptfull)

## `ptx_prepareTransaction`

### Parameters

0. `transaction`: [`TransactionInput`](../types/transactioninput.md#transactioninput)

### Returns

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

## `ptx_prepareTransactions`

### Parameters

0. `transactions`: [`TransactionInput[]`](../types/transactioninput.md#transactioninput)

### Returns

0. `transactionIds`: [`UUID[]`](../types/simpletypes.md#uuid)

## `ptx_queryPreparedTransactions`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

|Field           |Type                                             |
|----------------|-------------------------------------------------|
|`id`            |[`UUID`](../types/simpletypes.md#uuid)           |
|`created`       |[`Timestamp`](../types/simpletypes.md#timestamp) |


### Returns

0. `preparedTransactions`: [`PreparedTransaction[]`](../types/preparedtransaction.md#preparedtransaction)

## `ptx_queryStoredABIs`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

|Field           |Type                                             |
|----------------|-------------------------------------------------|
|`id`            |[`UUID`](../types/simpletypes.md#uuid)           |
|`created`       |[`Timestamp`](../types/simpletypes.md#timestamp) |

### Returns

0. `storedABIs`: [`StoredABI[]`](../types/storedabi.md#storedabi)

## `ptx_queryTransactionReceipts`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

|Field           |Type                                               |
|----------------|---------------------------------------------------|
|`id`              |[`UUID`](../types/simpletypes.md#uuid)           |
|`indexed`         |[`Timestamp`](../types/simpletypes.md#timestamp) |
|`success`         |`Boolean`                                        |
|`transactionHash` |[`HexBytes`](../types/simpletypes.md#hexbytes)   |
|`blockNumber`     |[`HexUInt64`](../types/simpletypes.md#hexuint64) |

### Returns

0. `receipts`: [`TransactionReceipt[]`](../types/transactionreceipt.md#transactionreceipt)

## `ptx_queryTransactions`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

|Field            |Type                                                |
|-----------------|----------------------------------------------------|
|`id`             |[`UUID`](../types/simpletypes.md#uuid)              |
|`idempotencyKey` |`String`                                            |
|`submitMode`     |`String`                                            |
|`created`        |[`Timestamp`](../types/simpletypes.md#timestamp)    |
|`abiReference`   |[`Timestamp`](../types/simpletypes.md#timestamp)    |
|`functionName`   |`String`                                            |
|`domain`         |`String`                                            |
|`from`           |`String`                                            |
|`to`             |[`HexBytes`](../types/simpletypes.md#hexbytes)      |

### Returns

0. `transactions`: [`Transaction[]`](../types/transaction.md#transaction)

## `ptx_queryTransactionsFull`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

|Field            |Type                                                |
|-----------------|----------------------------------------------------|
|`id`             |[`UUID`](../types/simpletypes.md#uuid)              |
|`idempotencyKey` |`String`                                            |
|`submitMode`     |`String`                                            |
|`created`        |[`Timestamp`](../types/simpletypes.md#timestamp)    |
|`abiReference`   |[`Timestamp`](../types/simpletypes.md#timestamp)    |
|`functionName`   |`String`                                            |
|`domain`         |`String`                                            |
|`from`           |`String`                                            |
|`to`             |[`HexBytes`](../types/simpletypes.md#hexbytes)      |

### Returns

0. `transactions`: [`TransactionFull[]`](../types/transactionfull.md#transactionfull)

## `ptx_resolveVerifier`

### Parameters

0. `keyIdentifier`: `string`
1. `algorithm`: `string`
2. `verifierType`: `string`

### Returns

0. `verifier`: `string`

## `ptx_sendTransaction`

### Parameters

0. `transaction`: [`TransactionInput`](../types/transactioninput.md#transactioninput)

### Returns

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

## `ptx_sendTransactions`

### Parameters

0. `transactions`: [`TransactionInput[]`](../types/transactioninput.md#transactioninput)

### Returns

0. `transactionIds`: [`UUID[]`](../types/simpletypes.md#uuid)

## `ptx_storeABI`

### Parameters

0. `abi`: [`Entry[]`](../types/transactioninput.md#entry)

### Returns

0. `storedABI`: [`StoredABI`](../types/storedabi.md#storedabi)

