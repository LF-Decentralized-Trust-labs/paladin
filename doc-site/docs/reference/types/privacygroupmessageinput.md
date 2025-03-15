---
title: PrivacyGroupMessageInput
---
{% include-markdown "./_includes/privacygroupmessageinput_description.md" %}

### Example

```json
{
    "domain": "",
    "group": "0x"
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `correlationId` | Optional UUID to designate a message as being in response to a previous message | [`UUID`](simpletypes.md#uuid) |
| `domain` | Domain of the privacy group | `string` |
| `group` | Group ID of the privacy group. All members in the group will receive a copy of the message (no guarantee of order) | [`HexBytes`](simpletypes.md#hexbytes) |
| `topic` | A topic for the message, which by convention should be a dot or slash separated string instructing the receiver how the message should be processed | `string` |
| `data` | Application defined JSON payload for the message. Can be any JSON type including as an object, array, hex string, other string, or number | [`RawJSON`](simpletypes.md#rawjson) |

