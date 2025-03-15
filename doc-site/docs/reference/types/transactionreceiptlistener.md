---
title: TransactionReceiptListener
---
{% include-markdown "./_includes/transactionreceiptlistener_description.md" %}

### Example

```json
{
    "name": "",
    "created": 0,
    "started": null,
    "filters": {},
    "options": {
        "domainReceipts": false
    }
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `name` | Unique name for the receipt listener | `string` |
| `created` | Time the listener was created | [`Timestamp`](simpletypes.md#timestamp) |
| `started` | If the listener is started - can be set to false to disable delivery server-side | `bool` |
| `filters` | Filters to apply to receipts | [`TransactionReceiptFilters`](transactionreceiptfilters.md#transactionreceiptfilters) |
| `options` | Options for the receipt listener | [`TransactionReceiptListenerOptions`](transactionreceiptlisteneroptions.md#transactionreceiptlisteneroptions) |

