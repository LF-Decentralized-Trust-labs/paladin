---
title: KeyMappingAndVerifier
---
{% include-markdown "./_includes/keymappingandverifier_description.md" %}

### Example

```json
{
    "verifier": null
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `identifier` | The full identifier used to look up this key | `string` |
| `wallet` | The name of the wallet containing this key | `string` |
| `keyHandle` | The handle within the wallet containing the key | `string` |
| `path` | The full path including the leaf that is the identifier | [`KeyPathSegment[]`](#keypathsegment) |
| `verifier` | The verifier associated with this key mapping | [`KeyVerifier`](#keyverifier) |

## KeyPathSegment

| Field Name | Description | Type |
|------------|-------------|------|
| `name` | The name of the path segment | `string` |
| `index` | The index of the path segment | `int64` |


## KeyVerifier

| Field Name | Description | Type |
|------------|-------------|------|
| `verifier` | The verifier value | `string` |
| `type` | The type of verifier | `string` |
| `algorithm` | The algorithm used by the verifier | `string` |


