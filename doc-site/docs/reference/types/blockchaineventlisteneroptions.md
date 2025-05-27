---
title: BlockchainEventListenerOptions
---
{% include-markdown "./_includes/blockchaineventlisteneroptions_description.md" %}

### Example

```json
{}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `batchSize` | The maximum number of events to deliver in each batch | `int` |
| `batchTimeout` | The maximum time to wait for a batch to fill before delivering | `string` |
| `fromBlock` | The block number from which to start listenening for events, or 'latest' to start from the latest block | `uint8[]` |

