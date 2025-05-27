---
title: BlockchainEventListenerStatus
---
{% include-markdown "./_includes/blockchaineventlistenerstatus_description.md" %}

### Example

```json
{
    "catchup": false,
    "checkpoint": {
        "blockNumber": 0
    }
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `catchup` | Whether the event listener is catching up to the latest block | `bool` |
| `checkpoint` | The checkpoint for the event listener | [`BlockchainEventListenerCheckpoint`](blockchaineventlistenercheckpoint.md#blockchaineventlistenercheckpoint) |

