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
| `incompleteStateReceiptBehavior` | When set to 'block_contract', if a transaction with incomplete state data is detected then delivery of all receipts on that individual smart contract address will pause until the missing state arrives. Receipts for other contract addresses continue to be delivered | `"block_contract", "process"` |

