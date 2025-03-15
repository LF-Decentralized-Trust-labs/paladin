---
title: ReliableMessageAck
---
{% include-markdown "./_includes/reliablemessageack_description.md" %}

### Example

```json
{
    "messageId": "00000000-0000-0000-0000-000000000000"
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `messageId` | ID of the reliable message delivery that this ack is associated with | [`UUID`](simpletypes.md#uuid) |
| `time` | Time the ack was received (or generated if it is local failure that stops a delivery being attempted) | [`Timestamp`](simpletypes.md#timestamp) |
| `error` | A permanent failure (a 'nack') that will stop any further attempts to deliver this message | `string` |

