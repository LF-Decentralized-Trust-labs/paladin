---
title: PublicTx
---
{% include-markdown "./_includes/publictx_description.md" %}

### Example

```json
{
    "from": "0x0000000000000000000000000000000000000000",
    "nonce": null,
    "created": 0,
    "transactionHash": null
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `localId` | A locally generated numeric ID for the public transaction. Unique within the node | `uint64` |
| `to` | The target contract address (optional) | [`EthAddress`](simpletypes.md#ethaddress) |
| `data` | The pre-encoded calldata (optional) | [`HexBytes`](simpletypes.md#hexbytes) |
| `from` | The sender's Ethereum address | [`EthAddress`](simpletypes.md#ethaddress) |
| `nonce` | The transaction nonce | [`HexUint64`](simpletypes.md#hexuint64) |
| `created` | The creation time | [`Timestamp`](simpletypes.md#timestamp) |
| `completedAt` | The completion time (optional) | [`Timestamp`](simpletypes.md#timestamp) |
| `transactionHash` | The transaction hash (optional) | [`Bytes32`](simpletypes.md#bytes32) |
| `success` | The transaction success status (optional) | `bool` |
| `revertData` | The revert data (optional) | [`HexBytes`](simpletypes.md#hexbytes) |
| `submissions` | The submission data (optional) | [`PublicTxSubmissionData[]`](#publictxsubmissiondata) |
| `activity` | The transaction activity records (optional) | [`TransactionActivityRecord[]`](#transactionactivityrecord) |
| `gas` | The gas limit for the transaction (optional) | [`HexUint64`](simpletypes.md#hexuint64) |
| `value` | The value transferred in the transaction (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `maxPriorityFeePerGas` | The maximum priority fee per gas (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `maxFeePerGas` | The maximum fee per gas (optional) | [`HexUint256`](simpletypes.md#hexuint256) |

## PublicTxSubmissionData

| Field Name | Description | Type |
|------------|-------------|------|
| `time` | The submission time | [`Timestamp`](simpletypes.md#timestamp) |
| `transactionHash` | The transaction hash | [`Bytes32`](simpletypes.md#bytes32) |
| `maxPriorityFeePerGas` | The maximum priority fee per gas (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `maxFeePerGas` | The maximum fee per gas (optional) | [`HexUint256`](simpletypes.md#hexuint256) |


## TransactionActivityRecord

| Field Name | Description | Type |
|------------|-------------|------|
| `time` | Time the record occurred | [`Timestamp`](simpletypes.md#timestamp) |
| `message` | Activity message | `string` |


