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

Most public chains, whether they are an L1 or an L2, will use paid gas for all transactions. Paladin uses EIP1559 transactions and must set a suitable `maxBaseFeePerGas` and `maxPriorityFeePerGas` value in those transactions
in order for them to be mined into blocks. Currently this must be configured on the Paladin node, for example:

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

### Automatic gas price retrieval

TBC

## Examples

The examples deploy contracts and invoke transactions before waiting for receipts to confirm they have succeesfully been mined. The default wait time in the Paladin Node SDK is 5 seconds. For many
public chains this will need increasing to a suitable value, based on the block period of the chain. The following is an example of a modification to a `waitForReceipt()` call:

```
const cashToken = await notoFactory
    .newNoto(verifierNode1, {
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
