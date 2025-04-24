---
title: BlockchainEventListener
---
{% include-markdown "./_includes/blockchaineventlistener_description.md" %}

### Example

```json
{
    "name": "",
    "created": 0,
    "started": null,
    "sources": null,
    "options": {}
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `name` | Unique name for the blockchain event listener | `string` |
| `created` | Time the listener was created | [`Timestamp`](simpletypes.md#timestamp) |
| `started` | If the listener is started - can be set to false to disable delivery server-side | `bool` |
| `sources` | Sources of events | [`BlockchainEventListenerSource[]`](blockchaineventlistenersource.md#blockchaineventlistenersource) |
| `options` | Options for the event listener | [`BlockchainEventListenerOptions`](blockchaineventlisteneroptions.md#blockchaineventlisteneroptions) |

