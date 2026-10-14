# Paladin Java SDK guidance

These instructions apply to `sdk/java/` in addition to the root `AGENTS.md`.
They capture durable conventions from the Java SDK's `CLAUDE.md`. That file, when
present locally, also contains historical planning notes; verify branch and issue
status before using those notes to resume work.

## SDK delivery timeline and order

Java SDK feature branches and PRs target `java-sdk-main`. Merge SDK work there;
do not target repository `main` directly. Before opening or reviewing an SDK PR,
confirm that its base branch is `java-sdk-main` so its commit list contains only
that feature's changes.

The following sequence comes from `sdk/java/CLAUDE.md` (the issue #1224 working
plan). Checkboxes describe local implementation as of October 2, 2026, not remote
issue or PR status. Items 1–6 are on `java-sdk-main`; item 7 is implemented on
`feat/java-sdk-websocket-subscriptions` and has not been merged. Dates are historical
targets; verify the branch, issues, and code before resuming work. Deliver each
enhancement in small, independently reviewable PRs. Update the checkboxes as
work lands.

### Phase 1 — Foundations (June 1–July 17, 2026)

1. [x] Set up the five-module Gradle project, register it in `settings.gradle`, and
   establish JDK 21, Jackson, and JUnit 5.
2. [x] Build core data models, primitives, the shared Jackson mapper, and
   serialization tests (#1230).
3. [x] Build the asynchronous HTTP JSON-RPC transport, retry and timeout handling,
   and the `PaladinException` hierarchy (#1231).
4. [x] Add the PTX and KeyManager RPC namespace clients.
5. [x] Add the fluent `TxBuilder`, `SentTransaction`, and receipt waiting. The
   current receipt wait uses polling; subscription support is in item 7.

### Phase 2 — Breadth (July 20–August 23, 2026; midterm August 24–31)

6. [x] Add the remaining RPC namespaces and the `PaladinClient` facade (#1239):
   registry/transport/blockindex, statestore, pgroup, then the facade. These
   clients and their tests are present in this checkout.
7. [x] Add WebSocket subscriptions and reconnection (#1240). The client supports
   PTX receipt and blockchain-event batches plus privacy-group messages, with
   acknowledgments, heartbeat, and automatic resubscription.
8. [ ] Add Noto, Pente, and Zeto domain helpers (#1241); split by domain if needed
   to keep PRs reviewable.
9. [ ] Add the testing module with mock clients, WireMock stubs, and Testcontainers
   support (#1242). The module currently contains only skeleton files.

### Phase 3 — Integration and docs (September 1–October 9, 2026)

10. [ ] Add a Testcontainers end-to-end suite against a real node. The
    `integration-test` module currently contains only a test package stub.
11. [ ] Complete public API Javadoc coverage for the finished SDK. Javadoc
    doclint is already enforced for the implemented modules; document new APIs
    as they are added.

### Phase 4 — Release (October 12–November 14, 2026; final review November 15–30)

12. [ ] Set up the release pipeline and publish version 0.1.0 to Maven Central,
    including a BOM and GPG signing. Confirm publishing account and key ownership
    before finalizing this step.

### Deferred work from the working notes

- [ ] Recover from idempotency-key conflicts in `TxBuilder.send()` by looking up
  the already-submitted transaction (the #1183 follow-up).
- [ ] Add `TxBuilder.call()` and `TxBuilder.prepare()`.
- [ ] Add local ABI encoding and the remaining builder conveniences (`Clone`,
  wrapping inputs, Solidity artifact support, and accessors).
- [ ] Add receipt waiting over WebSocket after item 7, keeping polling as a
  fallback; consider the full-receipt option from the TypeScript SDK.
- [x] Refresh `sdk/java/README.md` to describe the implemented clients and
  current quality gates (included in the WebSocket feature branch).

## Structure and API conventions

- Use JDK 21 and the `org.lfdt.paladin.sdk` group and package root.
- The five registered Gradle modules are `:sdk:java:core`, `:sdk:java:client`,
  `:sdk:java:domains`, `:sdk:java:testing`, and `:sdk:java:integration-test`.
- Put primitives, ABI models, query models, and RPC DTOs in `core`; HTTP transport,
  namespace clients, and transaction builders belong in `client`. Verify the
  implementation in the checkout before assuming planned modules are complete.
- Shared build conventions live in `sdk/java/build.gradle`; module build files
  declare their additional dependencies.
- Use Jackson with the shared `PaladinObjectMapper` for JSON handling. Preserve
  exact RPC method names, parameter order, JSON field names, and wire encodings.
- The HTTP transport uses JDK `java.net.http`; RPC calls return
  `CompletableFuture<T>`. Follow the existing unchecked `PaladinException` hierarchy
  for RPC, connection, timeout, and transaction errors.
- Consult `sdk/go/pkg/pldclient/`, `sdk/go/pkg/pldtypes/`, `sdk/go/pkg/pldapi/`, and
  `sdk/typescript/src/` for protocol context, and verify against the server contract.
  Write Java comments and Javadoc as standalone SDK documentation; avoid comments
  such as "mirror Go" or "same as TypeScript".
- For RPC DTOs, flatten embedded fields when the wire format is flat. Avoid
  combining `@JsonUnwrapped` with `@JsonCreator` on deserialized result types.
  Preserve explicitly nested objects where required by the RPC schema.
- State-store query methods must pass the trailing `StateStatusQualifier` in the
  correct position; test the wire parameters rather than copying client quirks.

## Quality gates

Run Gradle commands from the repository root:

```sh
./gradlew :sdk:java:core:test :sdk:java:client:test
./gradlew :sdk:java:core:test --tests 'org.lfdt.paladin.sdk.core.query.QueryBuilderTest'
./gradlew :sdk:java:core:check :sdk:java:client:check
./gradlew :sdk:java:core:spotlessApply :sdk:java:client:spotlessApply
```

Substitute the affected module paths when working elsewhere in the SDK. Run its
`check` task before considering a code change ready; `test` alone does not exercise
all quality gates.

- **Spotless:** standard Google Java Format (2 spaces), import ordering, unused
  import removal, final newlines, and the configured Apache-2.0 license header.
  Inspect formatter diffs and keep unrelated formatting changes out of the task.
- **Checkstyle:** declare unmodified parameters and local variables `final`, as
  required by `config/checkstyle/checkstyle.xml`; zero warnings are allowed.
- **Javadoc:** every public API member needs complete documentation, including
  constructors, factories, getters, and builder setters. `check` runs Javadoc with
  `-Xdoclint:all -Werror`. When preparing a commit series, keep the Javadoc pass in
  a dedicated follow-up commit, as requested in the SDK working notes; complete
  that pass and validate the final series before handing it off.
- **JaCoCo:** `check` enforces the instruction-coverage floor in
  `sdk/java/build.gradle` (currently 0.94). Preserve the coverage ratchet; do not
  lower the threshold to accommodate new code.

Use JUnit 5. Add core DTO serialization/deserialization tests in `core` itself:
tests in `client` do not count toward the `core` coverage bundle. Cover meaningful
equality, defaults, invalid enum values, and error/parameter behavior as appropriate.
Review branch coverage as well as instruction coverage for DTOs. Avoid contrived
tests for unreachable catches or compiler-generated behavior solely to raise coverage.

Reports are under each module's `build/reports/tests/test/` and
`build/reports/jacoco/test/html/`; generated API docs are under `build/docs/javadoc/`.
