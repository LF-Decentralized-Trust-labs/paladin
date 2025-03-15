---
title: PrivacyGroup
---
{% include-markdown "./_includes/privacygroup_description.md" %}

### Example

```json
{
    "id": "0x",
    "domain": "",
    "created": 0,
    "name": "",
    "members": null,
    "properties": null,
    "configuration": null,
    "genesisSalt": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "genesisSchema": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "genesisTransaction": "00000000-0000-0000-0000-000000000000",
    "contractAddress": null
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | The ID of the group, which is the hash-derived ID of the genesis state (assured to be unique within the domain) | [`HexBytes`](simpletypes.md#hexbytes) |
| `domain` | The domain of the privacy group | `string` |
| `created` | The creation time | [`Timestamp`](simpletypes.md#timestamp) |
| `name` | Optional name for the privacy group, which is indexed for efficient query | `string` |
| `members` | The member list must be a set of fully qualified identity locators 'some.identity@node.name' | `string[]` |
| `properties` | Application specific properties for the privacy group | `` |
| `configuration` | Domain specific configuration options that define the behavior of the privacy group | `` |
| `genesisSalt` | The salt used in the genesis state to ensure uniqueness of the resulting state ID | [`Bytes32`](simpletypes.md#bytes32) |
| `genesisSchema` | The ID of the schema for the genesis state | [`Bytes32`](simpletypes.md#bytes32) |
| `genesisTransaction` | The ID of the genesis transaction for the privacy group, correlated with the receipt | [`UUID`](simpletypes.md#uuid) |
| `contractAddress` | Returns the deployed contract address from the receipt associated with the transaction. Unset until the transaction is confirmed | [`EthAddress`](simpletypes.md#ethaddress) |

