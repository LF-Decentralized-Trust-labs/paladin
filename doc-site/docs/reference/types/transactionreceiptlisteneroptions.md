---
title: TransactionReceiptListenerOptions
---
{% include-markdown "./_includes/transactionreceiptlisteneroptions_description.md" %}

### Example

```json
{
    "domainReceipts": false
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `domainReceipts` | When true, a full domain receipt will be generated for each event with complete state data | `bool` |
| `incompleteStateReceiptBehavior` | Controls delivery behavior when receipt state data is incomplete. 'block_contract' pauses delivery for each individual smart contract address when incomplete states are detected. 'process' delivers all receipts immediately, regardless of what private state data is available. 'complete_only' delivers receipts whenever the domain confirms all expected states are complete, without regard for strict ordering | `"block_contract", "process", "complete_only"` |

