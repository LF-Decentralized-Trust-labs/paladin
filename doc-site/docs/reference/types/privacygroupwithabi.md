---
title: PrivacyGroupWithABI
---
{% include-markdown "./_includes/privacygroupwithabi_description.md" %}

### Example

```json
{
    "genesisABI": null
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | The ID of the group, which is the hash-derived ID of the genesis state (assured to be unique within the domain) | [`HexBytes`](simpletypes.md#hexbytes) |
| `domain` | The domain of the privacy group | `string` |
| `created` | The creation time | [`Timestamp`](simpletypes.md#timestamp) |
| `members` | The member list, validated by the domain to match the genesis state on creation/receipt | `string[]` |
| `contractAddress` | Returns the deployed contract address from the receipt associated with the transaction. Unset until the transaction is confirmed | [`EthAddress`](simpletypes.md#ethaddress) |
| `genesis` | The genesis state data (as stored in the state manager) | [`RawJSON`](simpletypes.md#rawjson) |
| `genesisTransaction` | The ID of the genesis transaction for the privacy group, correlated with the receipt | [`UUID`](simpletypes.md#uuid) |
| `genesisSchema` | The ID of the schema for the genesis state | [`Bytes32`](simpletypes.md#bytes32) |
| `genesisSignature` | String summary of the genesis schema | `string` |
| `genesisABI` | The full ABI of the genesis schema - only returned when querying directly by ID | [`Parameter`](privacygroupinput.md#parameter) |

