# Paladin Configuration

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| blockIndexer | Block indexer configuration | [`BlockIndexerConfig`](#blockindexer) | - |
| blockchain | Blockchain client configuration | [`EthClientConfig`](#blockchain) | - |
| db | Database configuration | [`DBConfig`](#db) | - |
| debugServer | Debug server configuration | [`DebugServerConfig`](#debugserver) | - |
| domainManager | Domain manager configuration | [`DomainManagerConfig`](#domainmanager) | - |
| domains | Map of domain configurations | [`map[string][DomainConfig]`](#domains) | - |
| groupManager | Group manager configuration | [`GroupManagerConfig`](#groupmanager) | - |
| grpc | GRPC configuration for plugin manager | [`GRPCConfig`](#grpc) | - |
| identifierCache | Identifier cache configuration | [`CacheConfig`](#identifiercache) | - |
| identityResolver | Identity resolver configuration | [`IdentityResolverConfig`](#identityresolver) | - |
| log | Logging configuration | [`LogConfig`](#log) | - |
| metricsServer | Metrics server configuration | [`MetricsServerConfig`](#metricsserver) | - |
| nodeName | Node name for transport identification | `string` | - |
| peerInactivityTimeout | Timeout for peer inactivity detection | `string` | - |
| peerReaperInterval | Interval for peer reaper cleanup | `string` | - |
| privateTxManager | Private transaction manager configuration | [`PrivateTxManagerConfig`](#privatetxmanager) | - |
| publicTxManager | Public transaction manager configuration | [`PublicTxManagerConfig`](#publictxmanager) | - |
| registries | Map of registry configurations | [`map[string][RegistryConfig]`](#registries) | - |
| registryManager | Registry manager configuration | [`RegistryManagerConfig`](#registrymanager) | - |
| reliableMessageResend | Reliable message resend configuration | `string` | - |
| reliableMessageWriter | Reliable message writer configuration | [`FlushWriterConfig`](#reliablemessagewriter) | - |
| reliableScanRetry | Reliable scan retry configuration | [`RetryConfig`](#reliablescanretry) | - |
| rpcServer | RPC server configuration | [`RPCServerConfig`](#rpcserver) | - |
| sendQueueLen | Maximum length of send queue | `int` | - |
| sendRetry | Send retry configuration | [`RetryConfigWithMax`](#sendretry) | - |
| signingModules | Map of signing module configurations | [`map[string][SigningModuleConfig]`](#signingmodules) | - |
| startup | Startup configuration | [`StartupConfig`](#startup) | - |
| statestore | State store configuration | [`StateStoreConfig`](#statestore) | - |
| tempDir | Temporary directory path | `string` | - |
| transports | Map of transport configurations | [`map[string][TransportConfig]`](#transports) | - |
| txManager | Transaction manager configuration | [`TxManagerConfig`](#txmanager) | - |
| verifierCache | Verifier cache configuration | [`CacheConfig`](#verifiercache) | - |
| wallets | List of wallet configurations | [`[WalletConfig]`](#wallets) | - |

## blockIndexer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| blockPollingInterval | Interval for polling new blocks | `string` | `"10s"` |
| chainHeadCacheLen | Length of chain head cache | `int` | `50` |
| commitBatchSize | Number of blocks to commit in a batch | `int` | `50` |
| commitBatchTimeout | Timeout for batch commits | `string` | `"100ms"` |
| eventStreams | Event streams configuration | [`EventStreamsConfig`](#blockindexereventstreams) | - |
| fromBlock | Starting block number for indexing | `[uint8]` | - |
| ignoredTransactionTypes | Transaction types to ignore | `[int64]` | - |
| insertDBBatchSize | Batch size for database inserts | `int` | `5000` |
| requiredConfirmations | Number of confirmations required | `int` | `0` |
| retry | Retry configuration | [`RetryConfig`](#blockindexerretry) | - |

## blockIndexer.eventStreams

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| blockDispatchQueueLength | Length of block dispatch queue | `int` | `100` |
| catchupQueryPageSize | Page size for catch-up queries | `int` | `100` |

## blockIndexer.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | - |
| initialDelay | Initial delay before retry | `string` | - |
| maxDelay | Maximum delay between retries | `string` | - |

## blockchain

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| gasEstimateFactor | Factor to multiply gas estimates by | `float64` | `2.00` |
| http | HTTP client configuration | [`HTTPClientConfig`](#blockchainhttp) | - |
| ws | WebSocket client configuration | [`WSClientConfig`](#blockchainws) | - |

## blockchain.http

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| auth | HTTP authentication configuration | [`HTTPBasicAuthConfig`](#blockchainhttpauth) | - |
| connectionTimeout | Connection timeout | `string` | `"30s"` |
| httpHeaders | HTTP headers to include in requests | `map[string][any]` | - |
| requestTimeout | Request timeout | `string` | `"30s"` |
| retry | HTTP retry configuration | [`HTTPRetryConfig`](#blockchainhttpretry) | - |
| tls | TLS configuration | [`TLSConfig`](#blockchainhttptls) | - |
| url | HTTP client URL | `string` | - |

## blockchain.http.auth

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| password | Basic auth password | `string` | - |
| username | Basic auth username | `string` | - |

## blockchain.http.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| count | Number of retry attempts | `int` | `5` |
| enabled | Whether HTTP retry is enabled | `bool` | `false` |
| errorStatusCodes | Regex pattern for status codes to retry | `string` | - |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maximumDelay | Maximum delay between retries | `string` | `"30s"` |

## blockchain.http.tls

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| ca | CA certificate content | `string` | - |
| caFile | Path to CA certificate file | `string` | - |
| cert | Certificate content | `string` | - |
| certFile | Path to certificate file | `string` | - |
| clientAuth | Whether client authentication is required | `bool` | `false` |
| enabled | Whether TLS is enabled | `bool` | `false` |
| insecureSkipHostVerify | Whether to skip host verification | `bool` | `false` |
| key | Private key content | `string` | - |
| keyFile | Path to private key file | `string` | - |
| requiredDNAttributes | Required DN attributes for client certificates | `map[string][string]` | - |

## blockchain.ws

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| auth | HTTP authentication configuration | [`HTTPBasicAuthConfig`](#blockchainwsauth) | - |
| connectRetry | Retry configuration for WebSocket connections | [`RetryConfig`](#blockchainwsconnectretry) | - |
| connectionTimeout | WebSocket connection timeout | `string` | `"30s"` |
| connectionTimeout | Connection timeout | `string` | `"30s"` |
| heartbeatInterval | WebSocket heartbeat interval | `string` | `"15s"` |
| httpHeaders | HTTP headers to include in requests | `map[string][any]` | - |
| initialConnectAttempts | Number of initial connection attempts | `int` | `0` |
| readBufferSize | WebSocket read buffer size | `string` | `"16Kb"` |
| requestTimeout | Request timeout | `string` | - |
| retry | HTTP retry configuration | [`HTTPRetryConfig`](#blockchainwsretry) | - |
| tls | TLS configuration | [`TLSConfig`](#blockchainwstls) | - |
| url | HTTP client URL | `string` | - |
| writeBufferSize | WebSocket write buffer size | `string` | `"16Kb"` |

## blockchain.ws.auth

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| password | Basic auth password | `string` | - |
| username | Basic auth username | `string` | - |

## blockchain.ws.connectRetry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxDelay | Maximum delay between retries | `string` | `"30s"` |

## blockchain.ws.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| count | Number of retry attempts | `int` | - |
| enabled | Whether HTTP retry is enabled | `bool` | - |
| errorStatusCodes | Regex pattern for status codes to retry | `string` | - |
| initialDelay | Initial delay before retry | `string` | - |
| maximumDelay | Maximum delay between retries | `string` | - |

## blockchain.ws.tls

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| ca | CA certificate content | `string` | - |
| caFile | Path to CA certificate file | `string` | - |
| cert | Certificate content | `string` | - |
| certFile | Path to certificate file | `string` | - |
| clientAuth | Whether client authentication is required | `bool` | - |
| enabled | Whether TLS is enabled | `bool` | - |
| insecureSkipHostVerify | Whether to skip host verification | `bool` | - |
| key | Private key content | `string` | - |
| keyFile | Path to private key file | `string` | - |
| requiredDNAttributes | Required DN attributes for client certificates | `map[string][string]` | - |

## db

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| postgres | PostgreSQL specific configuration | [`PostgresConfig`](#dbpostgres) | - |
| sqlite | SQLite specific configuration | [`SQLiteConfig`](#dbsqlite) | - |
| type | Database type (postgres, sqlite) | `string` | - |

## db.postgres

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| autoMigrate | Whether to automatically run migrations | `bool` | - |
| connMaxIdleTime | Maximum time a connection can be idle | `string` | - |
| connMaxLifetime | Maximum lifetime of a connection | `string` | - |
| debugQueries | Whether to log SQL queries for debugging | `bool` | - |
| dsn | Database connection string (can have {{.ParamName}} for replacement from params) | `string` | - |
| dsnParams | Parameters for DSN replacement | [`map[string][DSNParamLocation]`](#dbpostgresdsnparams) | - |
| maxIdleConns | Maximum number of idle connections | `int` | - |
| maxOpenConns | Maximum number of open connections | `int` | - |
| migrationsDir | Directory containing migration files | `string` | - |
| statementCache | Whether to cache prepared statements | `bool` | - |

## db.postgres.dsnParams[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| file | File containing the parameter value | `string` | - |

## db.sqlite

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| autoMigrate | Whether to automatically run migrations | `bool` | - |
| connMaxIdleTime | Maximum time a connection can be idle | `string` | - |
| connMaxLifetime | Maximum lifetime of a connection | `string` | - |
| debugQueries | Whether to log SQL queries for debugging | `bool` | - |
| dsn | Database connection string (can have {{.ParamName}} for replacement from params) | `string` | - |
| dsnParams | Parameters for DSN replacement | [`map[string][DSNParamLocation]`](#dbsqlitedsnparams) | - |
| maxIdleConns | Maximum number of idle connections | `int` | - |
| maxOpenConns | Maximum number of open connections | `int` | - |
| migrationsDir | Directory containing migration files | `string` | - |
| statementCache | Whether to cache prepared statements | `bool` | - |

## db.sqlite.dsnParams[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| file | File containing the parameter value | `string` | - |

## debugServer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| address | Server address | `string` | `"127.0.0.1"` |
| cors | CORS configuration | [`CORSConfig`](#debugservercors) | - |
| defaultRequestTimeout | Default request timeout | `string` | `"2m"` |
| enabled | Whether debug server is enabled | `bool` | `false` |
| maxRequestTimeout | Maximum request timeout | `string` | `"10m"` |
| port | Server port | `int` | - |
| readTimeout | Read timeout | `string` | - |
| shutdownTimeout | Shutdown timeout | `string` | `"10s"` |
| tls | TLS configuration | [`TLSConfig`](#debugservertls) | - |
| writeTimeout | Write timeout | `string` | - |

## domainManager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| contractCache | Contract cache configuration | [`CacheConfig`](#domainmanagercontractcache) | - |

## domainManager.contractCache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | - |

## domains[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| allowSigning | Whether this domain allows signing | `bool` | `false` |
| config | Domain-specific configuration | `map[string][any]` | - |
| defaultGasLimit | Default gas limit for transactions | `uint64` | - |
| fixedSigningIdentity | Fixed signing identity for this domain | `string` | - |
| init | Domain initialization configuration | [`DomainInitConfig`](#domainsinit) | - |
| plugin | Domain plugin configuration | [`PluginConfig`](#domainsplugin) | - |
| registryAddress | Registry address for this domain | `string` | - |

## domains[].init

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| retry | Retry configuration for domain initialization | [`RetryConfig`](#domainsinitretry) | - |

## domains[].init.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxDelay | Maximum delay between retries | `string` | `"30s"` |

## domains[].plugin

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| class | Plugin class name | `string` | - |
| library | Plugin library path | `string` | - |
| type | Plugin type | `string` | - |

## groupManager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| cache | Group manager cache configuration | [`CacheConfig`](#groupmanagercache) | - |
| messageListeners | Message listeners configuration | [`MessageListeners`](#groupmanagermessagelisteners) | - |

## groupManager.cache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | `50` |

## groupManager.messageListeners

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| readPageSize | Page size for reading messages | `int` | `100` |
| retry | Retry configuration | [`RetryConfig`](#groupmanagermessagelistenersretry) | - |

## groupManager.messageListeners.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxDelay | Maximum delay between retries | `string` | `"30s"` |

## grpc

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| shutdownTimeout | Timeout for GRPC shutdown | `string` | - |

## identityResolver

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| verifierCache | Verifier cache configuration | [`CacheConfig`](#identityresolververifiercache) | - |

## identityResolver.verifierCache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | `1000` |

## keyManager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| identifierCache | Identifier cache configuration | [`CacheConfig`](#keymanageridentifiercache) | - |
| verifierCache | Verifier cache configuration | [`CacheConfig`](#keymanagerverifiercache) | - |

## keyManager.identifierCache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | - |

## keyManager.verifierCache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | - |

## log

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| disableColor | Forces color to be disabled, even if we detect a TTY | `bool` | `false` |
| file | Configure file based logging | [`LogFileConfig`](#logfile) | - |
| forceColor | Forces color to be enabled, even if we do not detect a TTY | `bool` | `false` |
| format | Sets the log format (simple, json) | `string` | `"simple"` |
| json | Configure json based logging | [`LogJSONConfig`](#logjson) | - |
| level | Sets the logging level (debug, info, warn, error) | `string` | `"info"` |
| output | Sets the output destination (stdout, stderr, file) | `string` | `"stderr"` |
| timeFormat | String format for timestamps | `string` | `"2006-01-02T15:04:05.000Z07:00"` |
| utc | Sets log timestamps to the UTC timezone | `bool` | `false` |

## log.file

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| compress | Compress sets whether to compress backups | `bool` | `true` |
| filename | Sets the log filename prefix | `string` | `"paladin.log"` |
| maxAge | Sets the maximum age at which to roll | `string` | `"24h"` |
| maxBackups | Sets the maximum number of old files to keep | `int` | `2` |
| maxSize | Sets the size to roll logs at a given size | `string` | `"100Mb"` |

## log.json

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| fileField | Configures the JSON key containing the calling file | `string` | `"file"` |
| funcField | Configures the JSON key containing the calling function | `string` | `"func"` |
| levelField | Configures the JSON key containing the log level | `string` | `"level"` |
| messageField | Configures the JSON key containing the log message | `string` | `"message"` |
| timestampField | Configures the JSON key containing the timestamp of the log | `string` | `"@timestamp"` |

## metricsServer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| address | Server address | `string` | `"127.0.0.1"` |
| cors | CORS configuration | [`CORSConfig`](#metricsservercors) | - |
| defaultRequestTimeout | Default request timeout | `string` | `"2m"` |
| enabled | Whether metrics server is enabled | `bool` | `false` |
| maxRequestTimeout | Maximum request timeout | `string` | `"10m"` |
| port | Server port | `int` | - |
| readTimeout | Read timeout | `string` | - |
| shutdownTimeout | Shutdown timeout | `string` | `"10s"` |
| tls | TLS configuration | [`TLSConfig`](#metricsservertls) | - |
| writeTimeout | Write timeout | `string` | - |

## privateTxManager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| preparedTransactionDistributer | Prepared transaction distributer configuration | [`DistributerConfig`](#privatetxmanagerpreparedtransactiondistributer) | - |
| requestTimeout | Request timeout | `string` | `"1s"` |
| sequencer | Sequencer configuration | [`PrivateTxManagerSequencerConfig`](#privatetxmanagersequencer) | - |
| stateDistributer | State distributer configuration | [`DistributerConfig`](#privatetxmanagerstatedistributer) | - |
| writer | Writer configuration | [`FlushWriterConfig`](#privatetxmanagerwriter) | - |

## privateTxManager.preparedTransactionDistributer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| acknowledgementWriter | Acknowledgement writer configuration | [`FlushWriterConfig`](#privatetxmanagerpreparedtransactiondistributeracknowledgementwriter) | - |
| receivedStateWriter | Received state writer configuration | [`FlushWriterConfig`](#privatetxmanagerpreparedtransactiondistributerreceivedstatewriter) | - |

## privateTxManager.preparedTransactionDistributer.acknowledgementWriter

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| batchMaxSize | Maximum batch size | `int` | - |
| batchTimeout | Timeout for batch operations | `string` | - |
| workerCount | Number of worker threads | `int` | - |

## privateTxManager.preparedTransactionDistributer.receivedStateWriter

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| batchMaxSize | Maximum batch size | `int` | - |
| batchTimeout | Timeout for batch operations | `string` | - |
| workerCount | Number of worker threads | `int` | - |

## privateTxManager.sequencer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| assembleRequestTimeout | Assemble request timeout | `string` | `"1s"` |
| evalInterval | Evaluation interval | `string` | `"5m"` |
| maxConcurrentProcess | Maximum concurrent processes | `int` | `500` |
| maxInflightTransactions | Maximum inflight transactions | `int` | `500` |
| maxPendingEvents | Maximum pending events | `int` | `500` |
| persistenceRetryTimeout | Persistence retry timeout | `string` | `"5s"` |
| roundRobinCoordinatorBlockRangeSize | Round robin coordinator block range size | `int` | `100` |
| staleTimeout | Stale timeout | `string` | `"10m"` |

## privateTxManager.stateDistributer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| acknowledgementWriter | Acknowledgement writer configuration | [`FlushWriterConfig`](#privatetxmanagerstatedistributeracknowledgementwriter) | - |
| receivedStateWriter | Received state writer configuration | [`FlushWriterConfig`](#privatetxmanagerstatedistributerreceivedstatewriter) | - |

## privateTxManager.stateDistributer.acknowledgementWriter

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| batchMaxSize | Maximum batch size | `int` | - |
| batchTimeout | Timeout for batch operations | `string` | - |
| workerCount | Number of worker threads | `int` | - |

## privateTxManager.stateDistributer.receivedStateWriter

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| batchMaxSize | Maximum batch size | `int` | - |
| batchTimeout | Timeout for batch operations | `string` | - |
| workerCount | Number of worker threads | `int` | - |

## privateTxManager.writer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| batchMaxSize | Maximum batch size | `int` | `100` |
| batchTimeout | Timeout for batch operations | `string` | `"25ms"` |
| workerCount | Number of worker threads | `int` | `10` |

## publicTxManager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| balanceManager | Balance manager configuration | [`BalanceManagerConfig`](#publictxmanagerbalancemanager) | - |
| gasLimit | Gas limit configuration | [`GasLimitConfig`](#publictxmanagergaslimit) | - |
| gasPrice | Gas price configuration | [`GasPriceConfig`](#publictxmanagergasprice) | - |
| manager | Manager configuration | [`PublicTxManagerManagerConfig`](#publictxmanagermanager) | - |
| orchestrator | Orchestrator configuration | [`PublicTxManagerOrchestratorConfig`](#publictxmanagerorchestrator) | - |

## publicTxManager.balanceManager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| cache | Balance manager cache configuration | [`CacheConfig`](#publictxmanagerbalancemanagercache) | - |

## publicTxManager.balanceManager.cache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | `100` |

## publicTxManager.gasLimit

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| gasEstimateFactor | Gas estimate factor | `float64` | `1.50` |

## publicTxManager.gasPrice

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| ethFeeHistory | ETH fee history configuration | [`EthFeeHistoryConfig`](#publictxmanagergaspriceethfeehistory) | - |
| fixedGasPrice | Fixed gas price configuration | [`FixedGasPricing`](#publictxmanagergaspricefixedgasprice) | - |
| gasOracleAPI | Gas oracle API configuration | [`GasOracleAPIConfig`](#publictxmanagergaspricegasoracleapi) | - |
| increasePercentage | Gas price increase percentage | `int` | `10` |
| maxFeePerGasCap | Maximum fee per gas cap | `string` | - |
| maxPriorityFeePerGasCap | Maximum priority fee per gas cap | `string` | - |

## publicTxManager.gasPrice.ethFeeHistory

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| baseFeeBufferFactor | Base fee buffer factor | `int` | `1` |
| cache | Gas price cache configuration | [`GasPriceCacheConfig`](#publictxmanagergaspriceethfeehistorycache) | - |
| historyBlockCount | History block count | `int` | `20` |
| priorityFeePercentile | Priority fee percentile | `int` | `85` |

## publicTxManager.gasPrice.ethFeeHistory.cache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| enabled | Whether caching is enabled | `bool` | `true` |
| refreshTime | Cache refresh time | `string` | `"30s"` |

## publicTxManager.gasPrice.fixedGasPrice

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| maxFeePerGas | Maximum fee per gas | `string` | - |
| maxPriorityFeePerGas | Maximum priority fee per gas | `string` | - |

## publicTxManager.gasPrice.gasOracleAPI

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| auth | HTTP authentication configuration | [`HTTPBasicAuthConfig`](#publictxmanagergaspricegasoracleapiauth) | - |
| body | Request body | `string` | - |
| cache | Gas price cache configuration | [`GasPriceCacheConfig`](#publictxmanagergaspricegasoracleapicache) | - |
| connectionTimeout | Connection timeout | `string` | - |
| httpHeaders | HTTP headers to include in requests | `map[string][any]` | - |
| method | HTTP method | `string` | `"GET"` |
| requestTimeout | Request timeout | `string` | - |
| responseTemplate | Response template | `string` | - |
| retry | HTTP retry configuration | [`HTTPRetryConfig`](#publictxmanagergaspricegasoracleapiretry) | - |
| tls | TLS configuration | [`TLSConfig`](#publictxmanagergaspricegasoracleapitls) | - |
| url | HTTP client URL | `string` | - |

## publicTxManager.gasPrice.gasOracleAPI.auth

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| password | Basic auth password | `string` | - |
| username | Basic auth username | `string` | - |

## publicTxManager.gasPrice.gasOracleAPI.cache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| enabled | Whether caching is enabled | `bool` | `true` |
| refreshTime | Cache refresh time | `string` | `"30s"` |

## publicTxManager.gasPrice.gasOracleAPI.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| count | Number of retry attempts | `int` | - |
| enabled | Whether HTTP retry is enabled | `bool` | - |
| errorStatusCodes | Regex pattern for status codes to retry | `string` | - |
| initialDelay | Initial delay before retry | `string` | - |
| maximumDelay | Maximum delay between retries | `string` | - |

## publicTxManager.gasPrice.gasOracleAPI.tls

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| ca | CA certificate content | `string` | - |
| caFile | Path to CA certificate file | `string` | - |
| cert | Certificate content | `string` | - |
| certFile | Path to certificate file | `string` | - |
| clientAuth | Whether client authentication is required | `bool` | - |
| enabled | Whether TLS is enabled | `bool` | - |
| insecureSkipHostVerify | Whether to skip host verification | `bool` | - |
| key | Private key content | `string` | - |
| keyFile | Path to private key file | `string` | - |
| requiredDNAttributes | Required DN attributes for client certificates | `map[string][string]` | - |

## publicTxManager.manager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| activityRecords | Activity records configuration | [`PublicTxManagerActivityRecordsConfig`](#publictxmanagermanageractivityrecords) | - |
| interval | Manager interval | `string` | `"5s"` |
| maxInFlightOrchestrators | Maximum inflight orchestrators | `int` | `50` |
| nonceCacheTimeout | Nonce cache timeout | `string` | `"1h"` |
| orchestratorIdleTimeout | Orchestrator idle timeout | `string` | `"1s"` |
| orchestratorStaleTimeout | Orchestrator stale timeout | `string` | `"5m"` |
| orchestratorSwapTimeout | Orchestrator swap timeout | `string` | `"10m"` |
| retry | Retry configuration | [`RetryConfig`](#publictxmanagermanagerretry) | - |
| submissionWriter | Submission writer configuration | [`FlushWriterConfig`](#publictxmanagermanagersubmissionwriter) | - |

## publicTxManager.manager.activityRecords

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | `1000` |
| entriesPerTransaction | Records per transaction | `int` | `25` |

## publicTxManager.manager.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxDelay | Maximum delay between retries | `string` | `"30s"` |

## publicTxManager.manager.submissionWriter

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| batchMaxSize | Maximum batch size | `int` | `50` |
| batchTimeout | Timeout for batch operations | `string` | `"75ms"` |
| workerCount | Number of worker threads | `int` | `5` |

## publicTxManager.orchestrator

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| interval | Orchestrator interval | `string` | `"5s"` |
| maxInFlight | Maximum inflight transactions | `int` | `500` |
| persistenceRetryTime | Persistence retry time | `string` | `"5s"` |
| resubmitInterval | Resubmit interval | `string` | `"5m"` |
| stageRetryTime | Stage retry time | `string` | `"10s"` |
| staleTimeout | Stale timeout | `string` | `"5m"` |
| submissionRetry | Submission retry configuration | [`RetryConfigWithMax`](#publictxmanagerorchestratorsubmissionretry) | - |
| timelineMaxEntries | Timeline logging maximum entries | `int` | `0` |
| unavailableBalanceHandler | Unavailable balance handler | `string` | - |

## publicTxManager.orchestrator.submissionRetry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `4.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxAttempts | Maximum number of retry attempts | `int` | `3` |
| maxDelay | Maximum delay between retries | `string` | `"10s"` |

## registries[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| config | Registry specific configuration | `map[string][any]` | - |
| init | Registry initialization configuration | [`RegistryInitConfig`](#registriesinit) | - |
| plugin | Registry plugin configuration | [`PluginConfig`](#registriesplugin) | - |
| transports | Registry transports configuration | [`RegistryTransportsConfig`](#registriestransports) | - |

## registries[].init

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| retry | Retry configuration for registry initialization | [`RetryConfig`](#registriesinitretry) | - |

## registries[].init.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxDelay | Maximum delay between retries | `string` | `"30s"` |

## registries[].plugin

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| class | Plugin class name | `string` | - |
| library | Plugin library path | `string` | - |
| type | Plugin type | `string` | - |

## registries[].transports

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| enabled | Whether this registry is enabled for transport lookup | `bool` | `true` |
| hierarchySplitter | Character to split node names into hierarchy | `string` | - |
| propertyRegexp | Regular expression to match transport properties | `string` | `"^transport.(.*)$"` |
| requiredPrefix | Required prefix for node name matching | `string` | - |
| transportMap | Map from registry transport names to local transport names | `map[string][string]` | - |

## registryManager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| registryCache | Registry cache configuration | [`CacheConfig`](#registrymanagerregistrycache) | - |

## registryManager.registryCache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | - |

## reliableMessageWriter

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| batchMaxSize | Maximum batch size | `int` | - |
| batchTimeout | Timeout for batch operations | `string` | - |
| workerCount | Number of worker threads | `int` | - |

## reliableScanRetry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | - |
| initialDelay | Initial delay before retry | `string` | - |
| maxDelay | Maximum delay between retries | `string` | - |

## rpcServer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| http | HTTP server configuration | [`RPCServerConfigHTTP`](#rpcserverhttp) | - |
| ws | WebSocket server configuration | [`RPCServerConfigWS`](#rpcserverws) | - |

## rpcServer.http

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| address | Server address | `string` | - |
| cors | CORS configuration | [`CORSConfig`](#rpcserverhttpcors) | - |
| defaultRequestTimeout | Default request timeout | `string` | - |
| disabled | Whether HTTP server is disabled | `bool` | `false` |
| maxRequestTimeout | Maximum request timeout | `string` | - |
| port | Server port | `int` | - |
| readTimeout | Read timeout | `string` | - |
| shutdownTimeout | Shutdown timeout | `string` | - |
| staticServers | Static file server configurations | [`[StaticServerConfig]`](#rpcserverhttpstaticservers) | - |
| tls | TLS configuration | [`TLSConfig`](#rpcserverhttptls) | - |
| writeTimeout | Write timeout | `string` | - |

## rpcServer.http.cors

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| allowCredentials | Whether credentials are allowed | `bool` | - |
| allowedHeaders | List of allowed headers | `[string]` | - |
| allowedMethods | List of allowed methods | `[string]` | - |
| allowedOrigins | List of allowed origins | `[string]` | - |
| debug | Whether CORS debug mode is enabled | `bool` | - |
| enabled | Whether CORS is enabled | `bool` | - |
| maxAge | Maximum age for preflight requests | `string` | - |

## rpcServer.http.staticServers[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| baseRedirect | Redirect URL when hitting base path | `string` | - |
| enabled | Whether static server is enabled | `bool` | - |
| staticPath | Path to static files in server filesystem | `string` | - |
| urlPath | URL path to serve static files | `string` | - |

## rpcServer.http.tls

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| ca | CA certificate content | `string` | - |
| caFile | Path to CA certificate file | `string` | - |
| cert | Certificate content | `string` | - |
| certFile | Path to certificate file | `string` | - |
| clientAuth | Whether client authentication is required | `bool` | - |
| enabled | Whether TLS is enabled | `bool` | - |
| insecureSkipHostVerify | Whether to skip host verification | `bool` | - |
| key | Private key content | `string` | - |
| keyFile | Path to private key file | `string` | - |
| requiredDNAttributes | Required DN attributes for client certificates | `map[string][string]` | - |

## rpcServer.ws

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| address | Server address | `string` | - |
| cors | CORS configuration | [`CORSConfig`](#rpcserverwscors) | - |
| defaultRequestTimeout | Default request timeout | `string` | - |
| disabled | Whether WebSocket server is disabled | `bool` | `false` |
| maxRequestTimeout | Maximum request timeout | `string` | - |
| port | Server port | `int` | - |
| readBufferSize | Read buffer size for WebSocket connections | `string` | `"64KB"` |
| readTimeout | Read timeout | `string` | - |
| shutdownTimeout | Shutdown timeout | `string` | - |
| tls | TLS configuration | [`TLSConfig`](#rpcserverwstls) | - |
| writeBufferSize | Write buffer size for WebSocket connections | `string` | `"64KB"` |
| writeTimeout | Write timeout | `string` | - |

## rpcServer.ws.cors

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| allowCredentials | Whether credentials are allowed | `bool` | - |
| allowedHeaders | List of allowed headers | `[string]` | - |
| allowedMethods | List of allowed methods | `[string]` | - |
| allowedOrigins | List of allowed origins | `[string]` | - |
| debug | Whether CORS debug mode is enabled | `bool` | - |
| enabled | Whether CORS is enabled | `bool` | - |
| maxAge | Maximum age for preflight requests | `string` | - |

## rpcServer.ws.tls

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| ca | CA certificate content | `string` | - |
| caFile | Path to CA certificate file | `string` | - |
| cert | Certificate content | `string` | - |
| certFile | Path to certificate file | `string` | - |
| clientAuth | Whether client authentication is required | `bool` | - |
| enabled | Whether TLS is enabled | `bool` | - |
| insecureSkipHostVerify | Whether to skip host verification | `bool` | - |
| key | Private key content | `string` | - |
| keyFile | Path to private key file | `string` | - |
| requiredDNAttributes | Required DN attributes for client certificates | `map[string][string]` | - |

## sendRetry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | - |
| initialDelay | Initial delay before retry | `string` | - |
| maxAttempts | Maximum number of retry attempts | `int` | - |
| maxDelay | Maximum delay between retries | `string` | - |

## signingModules[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| config | Signing module specific configuration | `map[string][any]` | - |
| init | Signing module initialization configuration | [`SigningModuleInitConfig`](#signingmodulesinit) | - |
| plugin | Signing module plugin configuration | [`PluginConfig`](#signingmodulesplugin) | - |

## signingModules[].init

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| retry | Retry configuration for signing module initialization | [`RetryConfig`](#signingmodulesinitretry) | - |

## signingModules[].init.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxDelay | Maximum delay between retries | `string` | `"30s"` |

## signingModules[].plugin

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| class | Plugin class name | `string` | - |
| library | Plugin library path | `string` | - |
| type | Plugin type | `string` | - |

## startup

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| blockchainConnectRetry | Retry configuration for blockchain connection during startup | [`RetryConfigWithMax`](#startupblockchainconnectretry) | - |

## startup.blockchainConnectRetry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"500ms"` |
| maxAttempts | Maximum number of retry attempts | `int` | `10` |
| maxDelay | Maximum delay between retries | `string` | `"2s"` |

## statestore

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| schemaCache | Schema cache configuration | [`CacheConfig`](#statestoreschemacache) | - |

## statestore.schemaCache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | `1000` |

## transports[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| config | Transport specific configuration | `map[string][any]` | - |
| init | Transport initialization configuration | [`TransportInitConfig`](#transportsinit) | - |
| plugin | Transport plugin configuration | [`PluginConfig`](#transportsplugin) | - |

## transports[].init

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| retry | Retry configuration for transport initialization | [`RetryConfig`](#transportsinitretry) | - |

## transports[].init.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxDelay | Maximum delay between retries | `string` | `"30s"` |

## transports[].plugin

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| class | Plugin class name | `string` | - |
| library | Plugin library path | `string` | - |
| type | Plugin type | `string` | - |

## txManager

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| abi | ABI configuration | [`ABIConfig`](#txmanagerabi) | - |
| receiptListeners | Receipt listeners configuration | [`ReceiptListeners`](#txmanagerreceiptlisteners) | - |
| transactions | Transactions configuration | [`TransactionsConfig`](#txmanagertransactions) | - |

## txManager.abi

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| cache | ABI cache configuration | [`CacheConfig`](#txmanagerabicache) | - |

## txManager.abi.cache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | `100` |

## txManager.receiptListeners

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| readPageSize | Page size for reading receipts | `int` | `100` |
| retry | Retry configuration | [`RetryConfig`](#txmanagerreceiptlistenersretry) | - |
| stateGapCheckInterval | Interval for state gap checks | `string` | `"1s"` |

## txManager.receiptListeners.retry

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| factor | Exponential backoff factor | `float64` | `2.00` |
| initialDelay | Initial delay before retry | `string` | `"250ms"` |
| maxDelay | Maximum delay between retries | `string` | `"30s"` |

## txManager.transactions

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| cache | Transactions cache configuration | [`CacheConfig`](#txmanagertransactionscache) | - |

## txManager.transactions.cache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | `100` |

## wallets[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| keySelector | Regex pattern for key selection | `string` | `".*"` |
| keySelectorMustNotMatch | Whether to use non-matching regex pattern | `bool` | `false` |
| name | Name of the wallet | `string` | - |
| signer | Signer configuration (embedded only) | [`SignerConfig`](#walletssigner) | - |
| signerPluginName | Name of the signer plugin | `string` | - |
| signerType | Type of signer (embedded or plugin) | `string` | `"embedded"` |

## wallets[].signer

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| keyDerivation | Key derivation configuration | [`KeyDerivationConfig`](#walletssignerkeyderivation) | - |
| keyStore | Key store configuration | [`KeyStoreConfig`](#walletssignerkeystore) | - |

## wallets[].signer.keyDerivation

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| bip44DirectResolution | BIP44 direct resolution | `bool` | `false` |
| bip44HardenedSegments | BIP44 hardened segments | `int` | `1` |
| bip44Prefix | BIP44 prefix | `string` | `"m/44'/60'"` |
| seedKey | Seed key path | [`StaticKeyReference`](#walletssignerkeyderivationseedkey) | - |
| type | Key derivation type | `KeyDerivationType` | - |

## wallets[].signer.keyDerivation.seedKey

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| attributes | Key attributes | `map[string][string]` | - |
| index | Key index | `uint64` | `0` |
| keyHandle | Key handle | `string` | - |
| name | Key name | `string` | `"seed"` |
| path | Key path | [`[ConfigKeyPathEntry]`](#walletssignerkeyderivationseedkeypath) | - |

## wallets[].signer.keyDerivation.seedKey.path[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| index | Key path entry index | `uint64` | - |
| name | Key path entry name | `string` | - |

## wallets[].signer.keyStore

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| disableKeyListing | Whether to disable key listing | `bool` | `false` |
| filesystem | File system key store configuration | [`FileSystemKeyStoreConfig`](#walletssignerkeystorefilesystem) | - |
| keyStoreSigning | Whether key store signing is enabled | `bool` | `false` |
| static | Static key store configuration | [`StaticKeyStoreConfig`](#walletssignerkeystorestatic) | - |
| type | Key store type | `string` | - |

## wallets[].signer.keyStore.filesystem

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| cache | File system key store cache | [`CacheConfig`](#walletssignerkeystorefilesystemcache) | - |
| dirMode | File system key store directory mode | `string` | `"0700"` |
| fileMode | File system key store file mode | `string` | `"0600"` |
| path | File system key store path | `string` | `"keystore"` |

## wallets[].signer.keyStore.filesystem.cache

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| capacity | Cache capacity | `int` | `100` |

## wallets[].signer.keyStore.static

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| file | Static key store file | `string` | - |
| keys | Static key store keys | [`map[string][StaticKeyEntryConfig]`](#walletssignerkeystorestatickeys) | - |

## wallets[].signer.keyStore.static.keys[]

| Key | Description | Type | Default |
|-----|-------------|------|---------|
| encoding | Key entry encoding | `StaticKeyEntryEncoding` | - |
| filename | Key entry filename | `string` | - |
| inline | Inline key entry content | `string` | - |
| trim | Whether to trim key entry | `bool` | - |

