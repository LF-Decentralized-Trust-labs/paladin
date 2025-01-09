---
title: QueryJSON
---
{% include-markdown "./_includes/queryjson_description.md" %}

### Example

```json
{
    "eq": [
        {
            "field": "field1",
            "value": "abcde"
        },
        {
            "not": true,
            "caseInsensitive": true,
            "field": "field12",
            "value": "abcde"
        }
    ],
    "neq": [
        {
            "field": "field2",
            "value": "abcde"
        }
    ],
    "like": [
        {
            "field": "field3",
            "value": "abcde"
        }
    ],
    "lt": [
        {
            "field": "field4",
            "value": 12345
        }
    ],
    "lte": [
        {
            "field": "field5",
            "value": 12345
        }
    ],
    "gt": [
        {
            "field": "field6",
            "value": 12345
        }
    ],
    "gte": [
        {
            "field": "field7",
            "value": 12345
        }
    ],
    "in": [
        {
            "field": "field8",
            "values": [
                "abcde",
                "fghij"
            ]
        }
    ],
    "nin": [
        {
            "field": "field9",
            "values": [
                "abcde",
                "fghij"
            ]
        }
    ],
    "null": [
        {
            "not": true,
            "field": "field1"
        },
        {
            "field": "field2"
        }
    ],
    "limit": 10,
    "sort": [
        "field1 DESC",
        "field2"
    ]
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `or` | List of alternative statements | [`Statements[]`](#statements) |
| `equal` | Equal to | [`OpSingleVal[]`](#opsingleval) |
| `eq` | Equal to (short name) | [`OpSingleVal[]`](#opsingleval) |
| `neq` | Not equal to | [`OpSingleVal[]`](#opsingleval) |
| `like` | Like | [`OpSingleVal[]`](#opsingleval) |
| `lessThan` | Less than | [`OpSingleVal[]`](#opsingleval) |
| `lt` | Less than (short name) | [`OpSingleVal[]`](#opsingleval) |
| `lessThanOrEqual` | Less than or equal to | [`OpSingleVal[]`](#opsingleval) |
| `lte` | Less than or equal to (short name) | [`OpSingleVal[]`](#opsingleval) |
| `greaterThan` | Greater than | [`OpSingleVal[]`](#opsingleval) |
| `gt` | Greater than (short name) | [`OpSingleVal[]`](#opsingleval) |
| `greaterThanOrEqual` | Greater than or equal to | [`OpSingleVal[]`](#opsingleval) |
| `gte` | Greater than or equal to (short name) | [`OpSingleVal[]`](#opsingleval) |
| `in` | In | [`OpMultiVal[]`](#opmultival) |
| `nin` | Not in | [`OpMultiVal[]`](#opmultival) |
| `null` | Null | [`Op[]`](#op) |
| `limit` | Query limit | `int` |
| `sort` | Query sort order | `string[]` |

## Statements

| Field Name | Description | Type |
|------------|-------------|------|
| `or` | List of alternative statements | [`Statements[]`](#statements) |
| `equal` | Equal to | [`OpSingleVal[]`](#opsingleval) |
| `eq` | Equal to (short name) | [`OpSingleVal[]`](#opsingleval) |
| `neq` | Not equal to | [`OpSingleVal[]`](#opsingleval) |
| `like` | Like | [`OpSingleVal[]`](#opsingleval) |
| `lessThan` | Less than | [`OpSingleVal[]`](#opsingleval) |
| `lt` | Less than (short name) | [`OpSingleVal[]`](#opsingleval) |
| `lessThanOrEqual` | Less than or equal to | [`OpSingleVal[]`](#opsingleval) |
| `lte` | Less than or equal to (short name) | [`OpSingleVal[]`](#opsingleval) |
| `greaterThan` | Greater than | [`OpSingleVal[]`](#opsingleval) |
| `gt` | Greater than (short name) | [`OpSingleVal[]`](#opsingleval) |
| `greaterThanOrEqual` | Greater than or equal to | [`OpSingleVal[]`](#opsingleval) |
| `gte` | Greater than or equal to (short name) | [`OpSingleVal[]`](#opsingleval) |
| `in` | In | [`OpMultiVal[]`](#opmultival) |
| `nin` | Not in | [`OpMultiVal[]`](#opmultival) |
| `null` | Null | [`Op[]`](#op) |

## OpSingleVal

| Field Name | Description | Type |
|------------|-------------|------|
| `not` | Negate the operation | `bool` |
| `caseInsensitive` | Perform case-insensitive matching | `bool` |
| `field` | Field to apply the operation to | `string` |
| `value` | Value to compare against | [`RawJSON`](simpletypes.md#rawjson) |


## OpMultiVal

| Field Name | Description | Type |
|------------|-------------|------|
| `not` | Negate the operation | `bool` |
| `caseInsensitive` | Perform case-insensitive matching | `bool` |
| `field` | Field to apply the operation to | `string` |
| `values` | Values to compare against | [`RawJSON[]`](simpletypes.md#rawjson) |


## Op

| Field Name | Description | Type |
|------------|-------------|------|
| `not` | Negate the operation | `bool` |
| `caseInsensitive` | Perform case-insensitive matching | `bool` |
| `field` | Field to apply the operation to | `string` |



