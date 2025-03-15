---
title: PrivacyGroupEVMTXInput
---
{% include-markdown "./_includes/privacygroupevmtxinput_description.md" %}

### Example

```json
{
    "publicTxOptions": {}
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `idempotencyKey` | The idempotencyKey to use in the resulting transaction submission | `string` |
| `domain` | The domain that manages the privacy group | `string` |
| `group` | The privacy group ID | [`HexBytes`](simpletypes.md#hexbytes) |
| `from` | The local signing identity to use to submit the transaction | `string` |
| `to` | The private EVM smart contract address to invoke, or null for an EVM smart contract deployment | [`EthAddress`](simpletypes.md#ethaddress) |
| `gas` | Gas limit for the transaction (optional) | [`HexUint64`](simpletypes.md#hexuint64) |
| `value` | Native gas token value to transfer in the transaction, if supported by the EVM privacy group domain (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `input` | An object or array of unencoded inputs, when an function ABI is supplied. Or a hex string containing pre-encoded function selector and ABI encoded inputs | [`RawJSON`](simpletypes.md#rawjson) |
| `function` | The ABI fragment/entry for the function to call. Do not supply the whole ABI array, just one object for the function/constructor. Omit when pre-encoded hex input is provided | [`Entry`](transactioninput.md#entry) |
| `bytecode` | For contract deployments to EVM privacy groups, the bytecode must be submitted separately to the constructor parameters (which are supplied as input) | [`HexBytes`](simpletypes.md#hexbytes) |
| `publicTxOptions` | The public transaction submission options to use in the resulting transaction submission | [`PublicTxOptions`](#publictxoptions) |

## PublicTxOptions

| Field Name | Description | Type |
|------------|-------------|------|
| `gas` | The gas limit for the transaction (optional) | [`HexUint64`](simpletypes.md#hexuint64) |
| `value` | The value transferred in the transaction (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `maxPriorityFeePerGas` | The maximum priority fee per gas (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `maxFeePerGas` | The maximum fee per gas (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `gasPrice` | The gas price (optional) | [`HexUint256`](simpletypes.md#hexuint256) |


