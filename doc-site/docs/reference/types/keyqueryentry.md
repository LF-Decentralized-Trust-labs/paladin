---
title: KeyQueryEntry
---
{% include-markdown "./_includes/keyqueryentry_description.md" %}

### Example

```json
{
    "isKey": false,
    "hasChildren": false,
    "parent": "",
    "path": "",
    "name": "",
    "index": 0,
    "wallet": "",
    "keyHandle": "",
    "verifiers": null
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `isKey` | Whether this is a key | `bool` |
| `hasChildren` | Whether this has children | `bool` |
| `parent` | The parent of this key | `string` |
| `path` | The path of this key | `string` |
| `name` | The name of this key | `string` |
| `index` | The index of this key | `int64` |
| `wallet` | The wallet of this key | `string` |
| `keyHandle` | The handle of this key | `string` |
| `verifiers` | The verifiers of this key | [`KeyVerifier[]`](keymappingandverifier.md#keyverifier) |

