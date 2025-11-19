# Running Paladin with a public L1 or L2 chain as the base ledger

## Introduction

Paladin supports any EVM compatible chain as the base ledger for public transactions. This includes using public chains that use paid gas, such as Ethereum Mainnet L1 chain or the Linea Mainnet L2 chain. There
are certain considerations needed to run Paladin using a paid gas base ledger.

## Indexing

A public chain will have a long chain history. Paladin indexes the base ledger in order to confirm new transactions have been included in blocks. By default Paladin indexes the base ledger from block `0`. For a public
chain this could take many days to index. Paladin only requires block history from the point at which Paladin tranasctions and contracts were deployed. To reduce the time to index the chain, use the `fromBlock` configuration
option under the `blockIndexer` configuration section. For example:

```yaml
blockIndexer:
    fromBlock: 9024200
```

Set the `fromBlock` to a recent block near the head of the base ledger chain you are using. Indexing should complete within a few minutes and Paladin can then confirm any new Paladin transactions that are mined into blocks.

## Required confirmations

For public chains, it's important to configure the `requiredConfirmations` setting to ensure transaction finality. The block indexer waits for a specified number of confirmations before marking a transaction as succeeded, which helps protect against chain reorganizations.

### Default behavior

By default, Paladin sets `requiredConfirmations` to `0`, meaning transactions are considered confirmed immediately when they appear in a block. This is suitable for private chains or test networks where chain reorganizations are rare.

### Public chain considerations

Public chains can experience chain reorganizations, especially during periods of network congestion or when multiple validators produce blocks at similar times. Setting `requiredConfirmations` to a higher value provides protection against these reorganizations.

### Configuration example

To set the required confirmations for a public chain, add the following to your configuration:

```yaml
blockIndexer:
  fromBlock: 9024200
  requiredConfirmations: 12
```

This configuration tells Paladin to wait for 12 block confirmations before considering a transaction as successfully confirmed. The block indexer will only process and mark transactions as succeeded after they have been included in a block that is at least 12 blocks behind the current chain head.

### Trade-offs

- **Higher confirmations**: More secure against reorganizations but slower transaction confirmation
- **Lower confirmations**: Faster confirmation but higher risk of transaction reversal due to reorganizations

Choose the number of confirmations based on your application's security requirements and the specific characteristics of the chain you're using.

## Gas price

Most public chains, whether they are an L1 or an L2, will use paid gas for all transactions. Paladin uses EIP1559 transactions and must set a suitable `maxFeePerGas` and `maxPriorityFeePerGas` value in those transactions
in order for them to be mined into blocks. Paladin provides several options for configuring gas pricing, from fixed values to dynamic retrieval based on network conditions.

### Fixed gas pricing (node-level)

You can configure fixed gas prices at the node level that will be used for all transactions unless overridden at the transaction level:

```yaml
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

```yaml
publicTxManager:
  gasPrice:
    ethFeeHistory:
      priorityFeePercentile: 85
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

### Gas oracle API pricing

Paladin can retrieve gas prices from external gas oracle APIs, which can provide more accurate or specialized gas price data than the standard `eth_feeHistory` method. This is particularly useful for chains with limited RPC capabilities or when using specialized gas price services.

```yaml
publicTxManager:
  gasPrice:
    gasOracleAPI:
      url: "https://api.example.com/gas"
      method: "GET"
      auth:
        username: "your-username"
        password: "your-password"
      httpHeaders:
        X-API-Key: "your-api-key-here"
      responseTemplate: |
        {
          "maxFeePerGas": "{{.maxFeePerGas}}",
          "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
        }
```

Configuration options:

- **`url`**: The HTTP endpoint URL for the gas oracle API
- **`method`**: The HTTP method to use for the request. Default: `"GET"`
- **`body`**: The request body to send with the request.
- **`responseTemplate`**: A Go template string that extracts gas price data from the API response. The template receives the JSON response as data and should output a JSON object with `maxFeePerGas` and `maxPriorityFeePerGas` fields
- **Authentication**: HTTP client authentication options are supported:
  - **`auth.username`** and **`auth.password`**: Basic authentication
  - **`httpHeaders`**: Custom HTTP headers (useful for API keys, bearer tokens, etc.)

The gas oracle API must:
- Return a JSON response containing gas price information
- Be accessible via HTTP requests (GET, POST, etc.)
- Return gas prices in either hexadecimal format (e.g., `"0x2FAF080"`) or decimal format (e.g., `"50000000"`)

#### Custom JSON/RPC methods

Gas oracle APIs can also be custom JSON/RPC methods provided by your base ledger node. This is useful when your node supports specialized gas price calculation methods that aren't part of the standard Ethereum JSON/RPC specification.

```yaml
publicTxManager:
  gasPrice:
    gasOracleAPI:
      url: "http://localhost:8545"  # Your base ledger node
      method: "POST"
      body: |
        {
          "jsonrpc": "2.0",
          "method": "custom_feeHistory",
          "params": [20, "latest", [25, 50, 75]],
          "id": 1
        }
      responseTemplate: |
        {
          "maxFeePerGas": "{{.result.maxFeePerGas}}",
          "maxPriorityFeePerGas": "{{.result.maxPriorityFeePerGas}}"
        }
```

**Important considerations for custom JSON/RPC methods:**

- **Authentication**: If your base ledger node requires authentication, you'll need to configure it in both the `blockIndexer` section (for standard RPC calls) and the `gasOracleAPI` section (for gas price calls)
- **Response format**: The template receives the full JSON/RPC response, so you may need to access nested fields like `{{.result.fieldName}}` instead of `{{.fieldName}}`

Example API responses that work with the template above:

**Hexadecimal format:**
```json
{
  "result": {
    "maxFeePerGas": "0x2FAF080",
    "maxPriorityFeePerGas": "0x3B9ACA0"
  }
}
```

**Decimal format:**
```json
{
  "result": {
    "maxFeePerGas": "50000000",
    "maxPriorityFeePerGas": "62500000"
  }
}
```

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

When a transaction is rejected for being underpriced, Paladin can automatically increase the gas price by a configurable percentage and resubmit the transaction. This mechanism applies to node-level fixed pricing, gas oracle API pricing, and dynamic pricing, but not to transaction-level fixed pricing.

```yaml
publicTxManager:
  gasPrice:
    increasePercentage: 10
```

- **`increasePercentage`**: Percentage increase to apply when a transaction is underpriced. Default: 10%
- This increase is applied to the previously submitted gas price, not the current network price
- The mechanism helps ensure transactions are eventually included even during periods of rapidly increasing gas prices

### Gas price caps

To protect against excessive gas spending, you can configure maximum caps for both `maxFeePerGas` and `maxPriorityFeePerGas`. These caps apply universally to all gas pricing methods:

```yaml
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
4. **Gas oracle API**: If a gas oracle API is configured, retrieve gas prices from the external service
5. **Dynamic pricing**: Use `eth_feeHistory` to calculate optimal gas prices

This priority system ensures that transaction-level overrides take precedence while providing multiple fallback mechanisms for automatic gas pricing.

### Gas price caching

Paladin includes a built-in caching mechanism for gas price data that can significantly improve performance when submitting multiple transactions. The cache is particularly valuable in high-throughput scenarios where multiple transactions are likely to be included in the same block.

#### How it works

The gas price cache stores the most recent gas price data and automatically refreshes it at configurable intervals. When a transaction needs gas pricing, Paladin first checks the cache before making external API calls or RPC requests.

#### Configuration

Both `ethFeeHistory` and `gasOracleAPI` pricing methods support caching:

```yaml
publicTxManager:
  gasPrice:
    ethFeeHistory:
      cache:
        enabled: true
        refreshTime: "30s"
    gasOracleAPI:
      url: "https://api.example.com/gas"
      responseTemplate: |
        {
          "maxFeePerGas": "{{.maxFeePerGas}}",
          "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
        }
      cache:
        enabled: true
        refreshTime: "30s"
```

**Cache configuration options:**

- **`enabled`**: Whether to enable caching for gas price data. Default: `true`
- **`refreshTime`**: How often to refresh the cached gas price data. Default: `"30s"`

#### Refresh time considerations

The refresh time should be carefully considered based on your chain's characteristics and transaction patterns:

- **Block period relationship**: Consider your chain's block period when setting the refresh time. For example, if your chain has 10-second blocks, a 30-second refresh time would cover 3 blocks, while a 60-second refresh time would cover 6 blocks.

- **Higher refresh periods**:
  - ✅ Reduce API calls and RPC load
  - ✅ Lower resource usage
  - ❌ May result in outdated gas price data
  - ❌ Transactions might be rejected for being underpriced

- **Lower refresh periods**:
  - ✅ More accurate, up-to-date gas prices
  - ✅ Better success rates for transaction inclusion
  - ❌ Increased API calls and resource usage
  - ❌ Higher costs for external gas oracle services

#### When caching is most valuable

The gas price cache provides the most benefit when:

- **High transaction throughput**: Multiple transactions are submitted frequently
- **Same-block inclusion**: Transactions are likely to be included in the same or nearby blocks
- **Stable network conditions**: Gas prices don't change rapidly between blocks
- **External API usage**: Using gas oracle APIs where each call has a cost

#### Cache behavior

- Only one cache is active at a time (gas oracle API takes precedence over eth fee history)
- Cache is shared across all transactions and signing addresses
- Cache refresh happens in the background without blocking transaction submission
- If cache refresh fails, the previous cached value continues to be used
- Cache is automatically disabled if the gas pricing method is not configured

## Examples

The examples deploy contracts and invoke transactions before waiting for receipts to confirm they have succeesfully been mined. The default wait time in the Paladin examples is 30 seconds. This should be sufficient for most public chains, but it can be increased by modifying (`DEFAULT_POLL_TIMEOUT` in the examples config file)[https://github.com/LFDT-Paladin/paladin/blob/main/examples/common/src/config.ts].

Note: the example code won't wait the full polling period if the contract deploy completes early, so setting a large value won't increase the example run time unncessarily.

## Funding signing addresses with gas tokens e.g. ETH

Paladin uses a number of signing keys for the different domains and for initial deployment of the Paladin contracts. It also uses a new signing key for every public transaction as part of its approach to
preventing transactions on the base ledger from being correlated with each other.

A Paladin node can be configured to use predetermined signing keys, for example by using a pre-configured HD wallet mnemonic from which all signing keys are derived.

It is necessary to ensure the addresses used by Paladin have sufficient gas tokens to fund transactions.
