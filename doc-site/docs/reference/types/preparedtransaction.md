---
title: PreparedTransaction
---
{% include-markdown "./_includes/preparedtransaction_description.md" %}

### Example

```json
{
    "states": {}
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | The ID of the original transaction that prepared this transaction, and will be confirmed by its submission to the blockchain | [`UUID`](simpletypes.md#uuid) |
| `domain` | The domain of the original transaction that prepared this transaction submission | `string` |
| `to` | The to address or the original transaction that prepared this transaction submission | [`EthAddress`](simpletypes.md#ethaddress) |
| `transaction` | The Paladin transaction definition that has been prepared for submission, with the ABI and function details resolved | [`TransactionInput`](transactioninput.md#transactioninput) |
| `metadata` | Domain specific additional information generated during prepare in addition to the states. Used particularly in atomic multi-party transactions to separate data that can be disclosed, away from the full transaction submission payload | [`RawJSON`](simpletypes.md#rawjson) |
| `states` | Details of all states of the original transaction that prepared this transaction submission | [`TransactionStates`](transactionstates.md#transactionstates) |

