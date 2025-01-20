---
title: TransactionReceiptFull
---
{% include-markdown "./_includes/transactionreceiptfull_description.md" %}

### Example

```json
{}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | Transaction ID | [`UUID`](simpletypes.md#uuid) |
| `indexed` | The time when this receipt was indexed by the node, providing a relative order of transaction receipts within this node (might be significantly after the timestamp of the block) | [`Timestamp`](simpletypes.md#timestamp) |
| `sequence` | A local order of this receipt compared to other receipts on the node, used for ordering of receipts when delivering to receipt listeners | `uint64` |
| `domain` | The domain that executed the transaction, for private transactions only | `string` |
| `success` | Transaction success status | `bool` |
| `transactionHash` | Transaction hash | [`Bytes32`](simpletypes.md#bytes32) |
| `blockNumber` | Block number | `int64` |
| `transactionIndex` | Transaction index | `int64` |
| `logIndex` | Log index | `int64` |
| `source` | Event source | [`EthAddress`](simpletypes.md#ethaddress) |
| `failureMessage` | Failure message - set if transaction reverted | `string` |
| `revertData` | Encoded revert data - if available | [`HexBytes`](simpletypes.md#hexbytes) |
| `contractAddress` | New contract address - to be used in the 'To' field for subsequent invoke transactions | [`EthAddress`](simpletypes.md#ethaddress) |
| `states` | The state receipt for the transaction (private transactions only) | [`TransactionStates`](transactionstates.md#transactionstates) |
| `domainReceipt` | The domain receipt for the transaction (private transaction only) | [`RawJSON`](simpletypes.md#rawjson) |
| `domainReceiptError` | Contains the error if it was not possible to obtain the domain receipt for a private transaction | `string` |

