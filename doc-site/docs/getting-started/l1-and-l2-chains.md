# Running Paladin with a public L1 or L2 chain as the base ledger

## Introduction

Paladin supports any EVM compatible chain as the base ledger for public transactions. This includes using public chains that use paid gas, such as Ethereum Mainnet L1 chain or the Linea Mainnet L2 chain. There
are certain considerations needed to run Paladin using a paid gas base ledger.

## Indexing

A public chain will have a long chain history. Paladin indexes the base ledger in order to confirm new transactions have been included in blocks. By default Paladin indexes the base ledger from block `0`. For a public
chain this could take many days to index. Paladin only requires block history from the point at which Paladin tranasctions and contracts were deployed. To reduce the time to index the chain, use the `fromBlock` configuration
option under the `blockIndexer` configuration section. For example:

```
blockIndexer:
    fromBlock: 9024200
```

Set the `fromBlock` to a recent block near the head of the base ledger chain you are using. Indexing should complete within a few minutes and Paladin can then confirm any new Paladin transactions that are mined into blocks.

## Gas price

Most public chains, whether they are an L1 or an L2, will use paid gas for all transactions. Paladin uses EIP1559 transactions and must set a suitable `maxFeePerGas` and `maxPriorityFeePerGas` value in those transactions
in order for them to be mined into blocks. Paladin provides several options for configuring gas pricing, from fixed values to dynamic retrieval based on network conditions.

### Fixed gas pricing (node-level)

You can configure fixed gas prices at the node level that will be used for all transactions unless overridden at the transaction level:

```
publicTxManager:
  gasPrice:
    fixedGasPrice:
      maxFeePerGas: 0xdd71e69a
      maxPriorityFeePerGas: 0x307f4e
```

The gas prices must be set according to the following rules:

  - `maxPriorityFeePerGas` >= the current value returned by `eth_maxPriorityFeePerGas`
    - To accommodate fluctuations in the priority fee it may be necessary to set a value that is 10-50% higher than the current priority fee for the chain.
  - `maxFeePerGas` >= `maxPriorityFeePerGas`
  - `maxFeePerGas` >= `baseFeePerGas` returned from `eth_getBlockByNumber` for a recent block
  - `maxFeePerGas` >= the minimum transaction pool gas fee for the chain

Note: setting the fixed gas price values too high could result in paying more gas tokens (e.g. ETH) than necessary for transactions.

### Dynamic gas pricing using eth_feeHistory

Paladin can automatically retrieve optimal gas prices using the `eth_feeHistory` RPC method, which provides historical fee data from the network. This approach adapts to network conditions and can be more cost-effective than fixed pricing.

```
publicTxManager:
  gasPrice:
    ethFeeHistory:
      tipPercentile: 85
      historyBlockCount: 20
      baseFeeBufferFactor: 1
      cache:
        enabled: true
```

Configuration options:

- **`priorityFeePercentile`** (0-100): The percentile of historical priority fees to use. Higher values result in higher priority fees and faster transaction inclusion. Default: 85
- **`historyBlockCount`**: Number of historical blocks to analyze for fee calculation. More blocks provide better averages but may be less responsive to recent changes. Default: 20
- **`baseFeeBufferFactor`**: Multiplier for the base fee to provide a buffer against base fee increases. Default: 1 (no buffer)
- **`cache.enabled`**: Whether to cache fee history results to reduce RPC calls. Default: true

### Fixed gas pricing (transaction-level)

You can override gas pricing for individual transactions by setting gas prices directly on the transaction. This disables the gas pricing engine for that specific transaction:

```go
// Go SDK example
txBuilder.PublicTxOptions(pldapi.PublicTxOptions{
    MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000), // 20 Gwei
    MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(2000000000),  // 2 Gwei
})
```

```typescript
// TypeScript SDK example
const tx = await client.sendTransaction({
    // ... transaction details
    maxFeePerGas: "0x4a817c800", // 20 Gwei
    maxPriorityFeePerGas: "0x77359400" // 2 Gwei
});
```

Transaction-level fixed gas pricing bypasses all automatic gas pricing logic and uses the exact values you specify.

### Gas price increase mechanism

When a transaction is rejected for being underpriced, Paladin can automatically increase the gas price by a configurable percentage and resubmit the transaction. This mechanism applies to both node-level fixed pricing and dynamic pricing, but not to transaction-level fixed pricing.

```
publicTxManager:
  gasPrice:
    increasePercentage: 10
```

- **`increasePercentage`**: Percentage increase to apply when a transaction is underpriced. Default: 10%
- This increase is applied to the previously submitted gas price, not the current network price
- The mechanism helps ensure transactions are eventually included even during periods of rapidly increasing gas prices

### Gas price caps

To protect against excessive gas spending, you can configure maximum caps for both `maxFeePerGas` and `maxPriorityFeePerGas`. These caps apply universally to all gas pricing methods:

```
publicTxManager:
  gasPrice:
    maxFeePerGasCap: 0x5d21dba00  # 25 Gwei cap
    maxPriorityFeePerGasCap: 0x77359400  # 2 Gwei cap
```

- **`maxFeePerGasCap`**: Maximum allowed `maxFeePerGas` value
- **`maxPriorityFeePerGasCap`**: Maximum allowed `maxPriorityFeePerGas` value
- Caps are applied after all other gas pricing calculations, including increases for underpriced transactions
- If a calculated gas price exceeds the cap, it will be reduced to the cap value
- Setting caps too low may result in transactions being rejected by the network

### Gas pricing priority

Paladin uses the following priority order when determining gas prices for a transaction:

1. **Zero gas price chain**: If the chain is detected as having zero gas prices, return zero values
2. **Transaction-level fixed pricing**: If gas prices are set on the individual transaction
3. **Node-level fixed pricing**: If fixed gas prices are configured at the node level
4. **Dynamic pricing**: Use `eth_feeHistory` to calculate optimal gas prices

This priority system ensures that transaction-level overrides take precedence while providing fallback mechanisms for automatic gas pricing.

## Examples

The examples deploy contracts and invoke transactions before waiting for receipts to confirm they have succeesfully been mined. The default wait time in the Paladin Node SDK is 5 seconds. For many
public chains this will need increasing to a suitable value, based on the block period of the chain. The following is an example of a modification to a `waitForReceipt()` call:

```
const cashToken = await notoFactory
    .newNoto(verifierNode1, {
      name: "NOTO",
      symbol: "NOTO",
      notary: verifierNode1,
      notaryMode: "basic",
    })
    .waitForDeploy(20000);  // Allow 20 seconds for the contract deploy to complete
```

Note: the example code won't wait the full 20 seconds if the contract deploy completes early, so setting a large value won't increase the example run time unncessarily.

## Funding signing addresses with gas tokens e.g. ETH

Paladin uses a number of signing keys for the different domains and for initial deployment of the Paladin contracts. It also uses a new signing key for every public transaction as part of its approach to
preventing transactions on the base ledger from being correlated with each other.

A Paladin node can be configured to use predetermined signing keys, for example by using a pre-configured HD wallet mnemonic from which all signing keys are derived.

It is necessary to ensure the addresses used by Paladin have sufficient gas tokens to fund transactions.

### Automatic funding of addresses

TBC
