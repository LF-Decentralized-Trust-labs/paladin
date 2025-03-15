---
title: State
---
{% include-markdown "./_includes/state_description.md" %}

### Example

```json
{
    "id": "0x",
    "created": 0,
    "domain": "",
    "schema": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "contractAddress": null,
    "data": null
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | The ID of the state, which is generated from the content per the rules of the domain, and is unique within the contract | [`HexBytes`](simpletypes.md#hexbytes) |
| `created` | Server-generated creation timestamp for this state (query only) | [`Timestamp`](simpletypes.md#timestamp) |
| `domain` | The name of the domain this state is managed by | `string` |
| `schema` | The ID of the schema for this state, which defines what fields it has and which are indexed for query | [`Bytes32`](simpletypes.md#bytes32) |
| `contractAddress` | The address of the contract that manages this state within the domain | [`EthAddress`](simpletypes.md#ethaddress) |
| `data` | The JSON formatted data for this state | [`RawJSON`](simpletypes.md#rawjson) |
| `confirmed` | The confirmation record, if this an on-chain confirmation has been indexed from the base ledger for this state | [`StateConfirmRecord`](stateconfirmrecord.md#stateconfirmrecord) |
| `read` | Read record, only returned when querying within an in-memory domain context to represent read-lock on a state from a transaction in that domain context | [`StateReadRecord`](#statereadrecord) |
| `spent` | The spend record, if this an on-chain spend has been indexed from the base ledger for this state | [`StateSpendRecord`](statespendrecord.md#statespendrecord) |
| `locks` | When querying states within a domain context running ahead of the blockchain assembling transactions for submission, this provides detail on locks applied to the state | [`StateLock[]`](statelock.md#statelock) |
| `nullifier` | Only set if nullifiers are being used in the domain, and a nullifier has been generated that is available for spending this state | [`StateNullifier`](#statenullifier) |

## StateReadRecord


## StateNullifier


