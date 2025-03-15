---
title: ReliableMessage
---
{% include-markdown "./_includes/reliablemessage_description.md" %}

### Example

```json
{
    "sequence": 0,
    "id": "00000000-0000-0000-0000-000000000000",
    "created": 0,
    "node": "",
    "messageType": "",
    "metadata": null
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `sequence` | Sequence number for the position of this message in the local database | `uint64` |
| `id` | UUID for this message. A separate message, with a separate ID, is allocated for each participant that will receive the message | [`UUID`](simpletypes.md#uuid) |
| `created` | The time this message was created | [`Timestamp`](simpletypes.md#timestamp) |
| `node` | The target node for this message to be delivered to | `string` |
| `messageType` | The type of the message. Each type has a different locally stored metadata schema, and an on-the-wire full payload format that can be built from the metadata on the source node | `"state", "receipt", "prepared_txn", "privacy_group", "privacy_group_message"` |
| `metadata` | The locally stored (on the source node) minimal data that allows the on-the-wire message to be built using other stored data | [`RawJSON`](simpletypes.md#rawjson) |
| `ack` | An ack (or nack with error) that has finalized this message delivery so it will not be retried | [`ReliableMessageAckNoMsgID`](#reliablemessageacknomsgid) |

## ReliableMessageAckNoMsgID

| Field Name | Description | Type |
|------------|-------------|------|
| `time` | Time the ack was received (or generated if it is local failure that stops a delivery being attempted) | [`Timestamp`](simpletypes.md#timestamp) |
| `error` | A permanent failure (a 'nack') that will stop any further attempts to deliver this message | `string` |


