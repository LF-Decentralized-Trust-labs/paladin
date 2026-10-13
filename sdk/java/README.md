# Paladin Java SDK

Native Java SDK for [Paladin](https://github.com/LFDT-Paladin/paladin) — enables enterprise
JVM applications to interact with a Paladin node using a Java native library.

The core models, asynchronous HTTP RPC client, seven RPC namespaces, transaction builder, and
WebSocket subscriptions are implemented. Domain helpers, test utilities, node integration tests,
and Maven publishing are still in progress.

## Modules

| Module             | Description                                      |
|--------------------|--------------------------------------------------|
| `core`             | Data models, primitives, and subscription batches |
| `client`           | HTTP RPC, namespace clients, transaction builder, and WebSocket subscriptions |
| `domains`          | Planned helpers for Noto, Pente, and Zeto        |
| `testing`          | Planned test utilities                           |
| `integration-test` | Planned end-to-end tests against a Paladin node  |

## Building

```bash
./gradlew :sdk:java:core:build
./gradlew :sdk:java:client:build
./gradlew :sdk:java:domains:build
./gradlew :sdk:java:testing:build
./gradlew :sdk:java:integration-test:build
```

Requires JDK 21+.

## WebSocket subscriptions

Use a WebSocket endpoint and subscribe with an existing Paladin listener name. Each batch must be
acknowledged after processing or negatively acknowledged to request redelivery. The client pings
the node, reconnects with backoff after a disconnect, and resubscribes using the same local handle;
the server subscription ID changes on each connection.

```java
WebSocketClientConfig config = WebSocketClientConfig.builder("ws://localhost:8549").build();
try (WebSocketSubscriptionClient ws = new WebSocketSubscriptionClient(config)) {
  WebSocketSubscriptionClient.Subscription subscription =
      ws.subscribeReceipts("my-receipts", event -> {
        TransactionReceiptBatch batch = event.resultAs(TransactionReceiptBatch.class);
        // Process batch.receipts() before acknowledging it.
        event.ack();
      }).join();
  // Keep the client open while consuming events; call subscription.unsubscribe() when done.
}
```

`subscribeBlockchainEvents` and `subscribeMessages` use `TransactionEventBatch` and
`PrivacyGroupMessageBatch`. Configure authentication with `WebSocketClientConfig.Builder.header`.
Set `reconnectDelay(Duration.ZERO)` to disable reconnection. An optional error handler constructor
reports connection failures and callback errors.

### Testing against local nodes

With the operator development cluster running, run the opt-in live test:

```bash
PALADIN_LIVE_TEST=true ./gradlew :sdk:java:client:test --tests '*WebSocketLiveTest' --rerun-tasks
```

The test uses HTTP/WebSocket port pairs `31548/31549`, `31648/31649`, and `31748/31749`.
It requires existing transaction receipts and contract-registration events. It creates temporary
listeners, checks typed batches and NACK redelivery, and cuts its own TCP proxy connection to
verify automatic resubscription and rejection of stale acknowledgments. It deletes its listeners
afterward. Where a local privacy group exists, it sends a uniquely tagged test message that remains
in that group's history; otherwise it checks only message subscription and unsubscription.
The test is skipped unless `PALADIN_LIVE_TEST=true`.

## Code quality

The build enforces formatting, Checkstyle, Javadoc completeness, and test coverage. All run as part of
`build`/`check`, so CI fails if any is violated.

### Formatting (Spotless + Google Java Format)

```bash
./gradlew :sdk:java:core:spotlessCheck   # verify formatting (runs in build)
./gradlew :sdk:java:core:spotlessApply   # auto-format your changes
```

Enforces Google Java Format (2-space), import ordering, and the Apache-2.0 license header.

### Javadoc (doclint)

```bash
./gradlew :sdk:java:core:javadoc         # lint Javadoc (runs in build/check)
```

Runs the Javadoc tool with `-Xdoclint:all -Werror` over the public API, so the build fails on
any missing or malformed documentation — e.g. an undocumented public method, or a missing
`@param`/`@return`/`@throws` tag. Document every public class, method, and field you add.
Modules that have no documentable types yet (only a `package-info.java`) are skipped until they
gain their first class.

Output: `core/build/docs/javadoc/index.html`.

### Test coverage (JaCoCo)

```bash
./gradlew :sdk:java:core:test            # runs tests + generates the report
./gradlew :sdk:java:core:jacocoTestCoverageVerification   # fails if below threshold
```

Report: `core/build/reports/jacoco/test/html/index.html`.
The build fails if instruction coverage drops below the configured minimum (currently 96%).

## Privacy-group transactions

Select a group returned by `client.privacyGroups()` and use the transaction builder
for deployments, invocations, and read-only calls:

```java
var sent = client.newTx()
    .privacyGroup(group)
    .from("alice@node1")
    .to(contractAddress)
    .abi(erc20Abi)
    .function("transfer(address,uint256)")
    .inputs(Map.of("to", recipientAddress, "value", 25))
    .send();
var receipt = sent.waitForReceipt().join(); // inspect receipt.success()

var balance = client.newTx()
    .privacyGroupId(group.id())
    .domain(group.domain())
    .from("alice@node1")
    .to(contractAddress)
    .abi(erc20Abi)
    .function("balanceOf")
    .inputs(Map.of("account", recipientAddress))
    .dataFormat("mode=array&number=string")
    .call().join();
```

For deployments, use `.constructor().bytecode(bytecode)` and supply the constructor
arguments with `.inputs(...)`. Group transactions are implicitly private. Function
invocations require an inline ABI; overloaded names must use a full signature.
`buildPrivacyGroup()` returns the privacy-group RPC body; `build()` remains the
ordinary transaction body builder. Gas, value, and fee setters configure the
base-ledger submission options. ABI references and dependencies are not supported
by the privacy-group RPC and are rejected by the builder.

### Live integration test

The Java SDK integration test exercises this API against a running default
three-node installation. It creates a group on nodes 1 and 2, deploys ERC20Simple,
mints and transfers tokens, checks balances on both members, and verifies that
node 3 does not receive the group. With JDK 21 and the nodes running, invoke it
explicitly from the repository root:

```sh
./gradlew :sdk:java:integration-test:operatorE2E
```

The task prepares the ERC20Simple artifact and connects to localhost ports
31548, 31648, and 31748. Ordinary Java SDK `test` and `check` tasks exclude
this live test.
