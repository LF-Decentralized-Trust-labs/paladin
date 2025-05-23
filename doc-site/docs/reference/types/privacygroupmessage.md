---
title: PrivacyGroupMessage
---
{% include-markdown "./_includes/privacygroupmessage_description.md" %}

### Example

```json
{
    "id": "00000000-0000-0000-0000-000000000000",
    "localSequence": 0,
    "sent": 0,
    "received": 0,
    "node": "",
    "domain": "",
    "group": "0x"
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | Unique UUID for each message - will be the same on all nodes that receive the message | [`UUID`](simpletypes.md#uuid) |
| `localSequence` | Local sequence number for the message, with the local database of the local node. Will not be the same on all nodes that receive the message | `uint64` |
| `sent` | Time the message was sent. Generated on the sending node | [`Timestamp`](simpletypes.md#timestamp) |
| `received` | Time the message was received. Generated by the receiving node (same as sent on the sending node) | [`Timestamp`](simpletypes.md#timestamp) |
| `node` | The node that originated the message | `string` |
| `correlationId` | Optional UUID to designate a message as being in response to a previous message | [`UUID`](simpletypes.md#uuid) |
| `domain` | Domain of the privacy group | `string` |
| `group` | Group ID of the privacy group. All members in the group will receive a copy of the message (no guarantee of order) | [`HexBytes`](simpletypes.md#hexbytes) |
| `topic` | A topic for the message, which by convention should be a dot or slash separated string instructing the receiver how the message should be processed | `string` |
| `data` | Application defined JSON payload for the message. Can be any JSON type including as an object, array, hex string, other string, or number | [`RawJSON`](simpletypes.md#rawjson) |

