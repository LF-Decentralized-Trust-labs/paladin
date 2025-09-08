---
title: TransactionReceiptFilters
---
{% include-markdown "./_includes/transactionreceiptfilters_description.md" %}

### Example

```json
{}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `sequenceAbove` | Only deliver receipts above a certain sequence (rather than from the beginning of indexing of the chain) | `uint64` |
| `type` | Only deliver receipts for one transaction type (public/private) | `Enum[github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi.TransactionType]` |
| `domain` | Only deliver receipts for an individual domain (only valid with type=private) | `string` |

