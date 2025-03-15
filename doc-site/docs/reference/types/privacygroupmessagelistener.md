---
title: PrivacyGroupMessageListener
---
{% include-markdown "./_includes/privacygroupmessagelistener_description.md" %}

### Example

```json
{
    "name": "",
    "created": 0,
    "started": null,
    "filters": {},
    "options": {}
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `name` | Unique name for the message listener | `string` |
| `created` | Time the listener was created | [`Timestamp`](simpletypes.md#timestamp) |
| `started` | If the listener is started - can be set to false to disable delivery server-side | `bool` |
| `filters` | Filters to apply to messages | [`PrivacyGroupMessageListenerFilters`](#privacygroupmessagelistenerfilters) |
| `options` | Options for the receipt listener | [`PrivacyGroupMessageListenerOptions`](#privacygroupmessagelisteneroptions) |

## PrivacyGroupMessageListenerFilters

| Field Name | Description | Type |
|------------|-------------|------|
| `sequenceAbove` | Only deliver message above a certain sequence (rather than from the earliest message) | `uint64` |
| `domain` | Only deliver messages for an individual domain | `string` |
| `group` | Only deliver messages for an individual group ID | [`HexBytes`](simpletypes.md#hexbytes) |
| `topic` | Regular expression filter to apply to the topic of each message to determine whether to deliver it to the listener | `string` |


## PrivacyGroupMessageListenerOptions

| Field Name | Description | Type |
|------------|-------------|------|
| `excludeLocal` | When true, messages sent by the local node will not be delivered to the listener | `bool` |


