/*
 * Copyright contributors to Paladin, an LFDT project
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
package org.lfdt.paladin.sdk.client.tx;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import java.math.BigInteger;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.stream.Collectors;
import org.lfdt.paladin.sdk.client.exception.PaladinInvalidTransactionException;
import org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException;
import org.lfdt.paladin.sdk.client.privacygroup.PrivacyGroupClient;
import org.lfdt.paladin.sdk.client.ptx.PtxClient;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.AbiParameter;
import org.lfdt.paladin.sdk.core.abi.EntryType;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroup;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupEVMCall;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupEVMTXInput;
import org.lfdt.paladin.sdk.core.transaction.PublicTxOptions;
import org.lfdt.paladin.sdk.core.transaction.TransactionCall;
import org.lfdt.paladin.sdk.core.transaction.TransactionInput;
import org.lfdt.paladin.sdk.core.transaction.TransactionType;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;

/**
 * Fluent, high-level builder for constructing and submitting Paladin transactions. Privacy groups
 * selected with {@link #privacyGroup(PrivacyGroup)} or {@link #privacyGroupId(HexBytes)} route
 * submissions and calls through the privacy-group namespace. Use {@link #buildPrivacyGroup()} to
 * inspect their submission body; {@link #build()} is for ordinary public and private transactions.
 *
 * <p>It sits one level above {@link PtxClient}: instead of assembling a {@link TransactionInput}
 * and hand-rolling a receipt poll loop, describe the transaction by chaining, finish with {@link
 * #send()}, and wait on the handle it returns:
 *
 * <pre>{@code
 * TransactionReceipt receipt =
 *     ptx.newTx()
 *         .publicTx()
 *         .from("alice")
 *         .to("0x0102030405060708090a0b0c0d0e0f1011121314")
 *         .function("transfer")
 *         .abiJson(abiJson)
 *         .inputs(Map.of("to", "0x...", "amount", 100))
 *         .receiptTimeout(Duration.ofMinutes(1))
 *         .send()
 *         .waitForReceipt()
 *         .join();
 * }</pre>
 *
 * <h2>Deferred errors</h2>
 *
 * <p>No chaining call ever throws. Setters that can fail — parsing an address, ABI JSON, or
 * arbitrary input values — record the failure instead and return {@code this}, so a chain reads
 * uninterrupted and a malformed transaction surfaces at exactly one place. The first recorded error
 * wins and is replayed from {@link #build()} (thrown) and from every future on the {@link
 * SentTransaction} {@link #send()} returns, together with the structural checks in {@link
 * #build()}.
 *
 * <h2>Sending and waiting are separate</h2>
 *
 * <p>{@link #send()} submits and returns immediately, exactly as Go's {@code Send()} and
 * TypeScript's {@code sendTransaction()} do. Waiting is a second, explicit step on the returned
 * {@link SentTransaction}: {@link SentTransaction#waitForReceipt()} polls {@code
 * ptx_getTransactionReceipt} every {@link #pollingInterval(Duration)} (default one second) until a
 * receipt lands or {@link #receiptTimeout(Duration)} (default 30 seconds) elapses, at which point
 * the future fails with a {@link PaladinTimeoutException}. Take the transaction id alone, without
 * waiting, from {@link SentTransaction#id()}.
 *
 * <p>Instances are mutable and not thread-safe; build and send from a single thread. The returned
 * futures may be composed freely from any thread.
 */
public final class TxBuilder {

  /** Default delay between receipt polls, used unless {@link #pollingInterval(Duration)} is set. */
  public static final Duration DEFAULT_POLLING_INTERVAL = Duration.ofSeconds(1);

  /** Default receipt wait, used unless {@link #receiptTimeout(Duration)} is set. */
  public static final Duration DEFAULT_RECEIPT_TIMEOUT = Duration.ofSeconds(30);

  private final PtxClient ptx;
  private final PrivacyGroupClient privacyGroups;
  private HexBytes group;
  private String block;
  private String dataFormat;

  private String idempotencyKey;
  private TransactionType type;
  private String domain;
  private String function;
  private Bytes32 abiReference;
  private String from;
  private EthAddress to;
  private JsonNode data;
  private HexUint64 gas;
  private HexUint256 value;
  private HexUint256 maxPriorityFeePerGas;
  private HexUint256 maxFeePerGas;
  private final List<UUID> dependsOn = new ArrayList<>();
  private final List<AbiEntry> abi = new ArrayList<>();
  private HexBytes bytecode;

  private Duration pollingInterval = DEFAULT_POLLING_INTERVAL;
  private Duration receiptTimeout = DEFAULT_RECEIPT_TIMEOUT;

  private PaladinInvalidTransactionException deferredError;

  private TxBuilder(final PtxClient ptx, final PrivacyGroupClient privacyGroups) {
    this.ptx = Objects.requireNonNull(ptx, "ptx");
    this.privacyGroups = Objects.requireNonNull(privacyGroups, "privacyGroups");
  }

  /**
   * Starts a builder over an existing PTX client.
   *
   * @param ptx the PTX client used to submit and poll; must not be {@code null}
   * @return a new builder
   */
  public static TxBuilder on(final PtxClient ptx) {
    return Objects.requireNonNull(ptx, "ptx").newTx();
  }

  /**
   * Starts a builder over an RPC transport, wrapping it in a {@link PtxClient}.
   *
   * @param rpc the RPC transport to make calls on; must not be {@code null}
   * @return a new builder
   */
  public static TxBuilder on(final RpcClient rpc) {
    return on(new PtxClient(rpc), new PrivacyGroupClient(rpc));
  }

  /**
   * Starts a builder using namespace clients for submission and receipt retrieval.
   *
   * @param ptx the transaction client; must not be {@code null}
   * @param privacyGroups the privacy-group client for the same node; must not be {@code null}
   * @return a new builder
   */
  public static TxBuilder on(final PtxClient ptx, final PrivacyGroupClient privacyGroups) {
    return new TxBuilder(ptx, privacyGroups);
  }

  /**
   * Targets a privacy group, taking both its domain and identifier.
   *
   * @param group the group to execute in; must not be {@code null}
   * @return this builder; invalid input is deferred until a terminal operation
   */
  public TxBuilder privacyGroup(final PrivacyGroup group) {
    if (group == null) {
      defer("privacy group is required", null);
      return this;
    }
    return domain(group.domain()).privacyGroupId(group.id());
  }

  /**
   * Targets a privacy group by identifier. Set its domain separately with {@link #domain(String)}.
   * The transaction is implicitly private; an explicit public type is rejected.
   *
   * @param group the nonempty group identifier
   * @return this builder
   */
  public TxBuilder privacyGroupId(final HexBytes group) {
    if (group == null || group.isEmpty()) {
      defer("privacy group identifier must not be empty", null);
    } else {
      this.group = group;
    }
    return this;
  }

  /**
   * Targets a privacy group using a hex identifier. Parsing errors are deferred.
   *
   * @param group the hex group identifier
   * @return this builder
   */
  public TxBuilder privacyGroupId(final String group) {
    try {
      privacyGroupId(HexBytes.fromString(group));
    } catch (final RuntimeException e) {
      defer("invalid privacy group identifier", e);
    }
    return this;
  }

  /**
   * Sets the block context used by {@link #call()}.
   *
   * @param block a block number or special value such as {@code "latest"}
   * @return this builder
   */
  public TxBuilder block(final String block) {
    this.block = block;
    return this;
  }

  /**
   * Sets the output format used by {@link #call()}.
   *
   * @param dataFormat the node's ABI output serialization options
   * @return this builder
   */
  public TxBuilder dataFormat(final String dataFormat) {
    this.dataFormat = dataFormat;
    return this;
  }

  // ---------------------------------------------------------------------------------------------
  // Chaining setters — these never throw; failures are deferred to build()/send().
  // ---------------------------------------------------------------------------------------------

  /**
   * Sets the transaction type.
   *
   * @param type public (straight to the base ledger) or private (masked through a domain)
   * @return this builder
   */
  public TxBuilder type(final TransactionType type) {
    this.type = type;
    return this;
  }

  /**
   * Marks this as a public transaction, submitted directly to the base ledger.
   *
   * @return this builder
   */
  public TxBuilder publicTx() {
    return type(TransactionType.PUBLIC);
  }

  /**
   * Marks this as a private transaction, whose state is selectively disclosed through a Paladin
   * domain.
   *
   * @return this builder
   */
  public TxBuilder privateTx() {
    return type(TransactionType.PRIVATE);
  }

  /**
   * Sets the domain name, required for private transactions.
   *
   * @param domain the domain name
   * @return this builder
   */
  public TxBuilder domain(final String domain) {
    this.domain = domain;
    return this;
  }

  /**
   * Sets the function to invoke, by name or full signature.
   *
   * @param function the function name or signature
   * @return this builder
   */
  public TxBuilder function(final String function) {
    this.function = function;
    return this;
  }

  /**
   * Marks this as a deploy: clears the function and the target address.
   *
   * @return this builder
   */
  public TxBuilder constructor() {
    this.function = null;
    this.to = null;
    return this;
  }

  /**
   * Sets the reference to an ABI already stored on the node, as an alternative to supplying one
   * inline.
   *
   * @param abiReference the stored ABI hash reference
   * @return this builder
   */
  public TxBuilder abiReference(final Bytes32 abiReference) {
    this.abiReference = abiReference;
    return this;
  }

  /**
   * Sets the idempotency key, making submission exactly-once for a given business transaction.
   *
   * @param idempotencyKey the externally supplied unique identifier
   * @return this builder
   */
  public TxBuilder idempotencyKey(final String idempotencyKey) {
    this.idempotencyKey = idempotencyKey;
    return this;
  }

  /**
   * Sets the local signing identity that authorizes the transaction.
   *
   * @param from the signing identity locator
   * @return this builder
   */
  public TxBuilder from(final String from) {
    this.from = from;
    return this;
  }

  /**
   * Sets the target contract address.
   *
   * @param to the target contract address, or {@code null} for a deploy
   * @return this builder
   */
  public TxBuilder to(final EthAddress to) {
    this.to = to;
    return this;
  }

  /**
   * Sets the target contract address from its hex form. An unparseable address is deferred, not
   * thrown.
   *
   * @param to the target contract address as hex, with or without the {@code 0x} prefix
   * @return this builder
   */
  public TxBuilder to(final String to) {
    try {
      this.to = EthAddress.fromString(to);
    } catch (final RuntimeException e) {
      defer("invalid 'to' address: " + to, e);
    }
    return this;
  }

  /**
   * Sets deploy bytecode, required for public and privacy-group deploys. Other private deploys
   * obtain bytecode from their domain and reject this option.
   *
   * @param bytecode the deploy bytecode
   * @return this builder
   */
  public TxBuilder bytecode(final HexBytes bytecode) {
    this.bytecode = bytecode;
    return this;
  }

  /**
   * Sets the deploy bytecode from its hex form. Unparseable bytecode is deferred, not thrown.
   *
   * @param bytecode the deploy bytecode as hex, with or without the {@code 0x} prefix
   * @return this builder
   */
  public TxBuilder bytecode(final String bytecode) {
    try {
      this.bytecode = HexBytes.fromString(bytecode);
    } catch (final RuntimeException e) {
      defer("invalid bytecode", e);
    }
    return this;
  }

  /**
   * Adds transactions that must be mined (or deleted) before this one is submitted.
   *
   * @param dependencies the ids of the transactions depended on
   * @return this builder
   */
  public TxBuilder dependsOn(final UUID... dependencies) {
    this.dependsOn.addAll(Arrays.asList(dependencies));
    return this;
  }

  /**
   * Adds transactions that must be mined (or deleted) before this one is submitted.
   *
   * @param dependencies the ids of the transactions depended on
   * @return this builder
   */
  public TxBuilder dependsOn(final List<UUID> dependencies) {
    this.dependsOn.addAll(dependencies);
    return this;
  }

  /**
   * Adds entries to the inline ABI.
   *
   * @param abi the ABI entries to add
   * @return this builder
   */
  public TxBuilder abi(final List<AbiEntry> abi) {
    this.abi.addAll(abi);
    return this;
  }

  /**
   * Adds a single entry to the inline ABI.
   *
   * @param entry the ABI entry to add
   * @return this builder
   */
  public TxBuilder abiEntry(final AbiEntry entry) {
    this.abi.add(entry);
    return this;
  }

  /**
   * Adds entries to the inline ABI from a JSON array. Malformed JSON is deferred, not thrown.
   *
   * @param abiJson a JSON array of ABI entries
   * @return this builder
   */
  public TxBuilder abiJson(final String abiJson) {
    try {
      this.abi.addAll(List.of(PaladinObjectMapper.shared().readValue(abiJson, AbiEntry[].class)));
    } catch (final JsonProcessingException | RuntimeException e) {
      defer("invalid ABI JSON", e);
    }
    return this;
  }

  /**
   * Sets the call inputs from any value the SDK mapper can serialize — a {@code Map}, a POJO, a
   * {@code List} of positional arguments, or a {@link JsonNode}. A value that cannot be serialized
   * is deferred, not thrown.
   *
   * @param inputs the call inputs
   * @return this builder
   */
  public TxBuilder inputs(final Object inputs) {
    try {
      this.data = PaladinObjectMapper.shared().valueToTree(inputs);
    } catch (final RuntimeException e) {
      defer("invalid transaction inputs", e);
    }
    return this;
  }

  /**
   * Sets the call inputs from raw JSON — an object keyed by parameter name, or an array of
   * positional arguments. Malformed JSON is deferred, not thrown.
   *
   * @param inputsJson the call inputs as JSON
   * @return this builder
   */
  public TxBuilder inputsJson(final String inputsJson) {
    try {
      this.data = PaladinObjectMapper.shared().readTree(inputsJson);
    } catch (final JsonProcessingException | RuntimeException e) {
      defer("invalid transaction inputs JSON", e);
    }
    return this;
  }

  /**
   * Sets the gas limit, overriding the node's estimate.
   *
   * @param gas the gas limit
   * @return this builder
   */
  public TxBuilder gas(final long gas) {
    this.gas = HexUint64.of(gas);
    return this;
  }

  /**
   * Sets the native value to transfer with the transaction.
   *
   * @param value the native value to transfer
   * @return this builder
   */
  public TxBuilder value(final long value) {
    this.value = HexUint256.of(value);
    return this;
  }

  /**
   * Sets the native value to transfer with the transaction.
   *
   * @param value the native value to transfer
   * @return this builder
   */
  public TxBuilder value(final BigInteger value) {
    this.value = HexUint256.of(value);
    return this;
  }

  /**
   * Sets the EIP-1559 max fee per gas. Supplying it fixes gas pricing, disabling the node's
   * gas-pricing engine for this transaction.
   *
   * @param maxFeePerGas the max fee per gas
   * @return this builder
   */
  public TxBuilder maxFeePerGas(final BigInteger maxFeePerGas) {
    this.maxFeePerGas = HexUint256.of(maxFeePerGas);
    return this;
  }

  /**
   * Sets the EIP-1559 max priority fee per gas. Supplying it fixes gas pricing, disabling the
   * node's gas-pricing engine for this transaction.
   *
   * @param maxPriorityFeePerGas the max priority fee per gas
   * @return this builder
   */
  public TxBuilder maxPriorityFeePerGas(final BigInteger maxPriorityFeePerGas) {
    this.maxPriorityFeePerGas = HexUint256.of(maxPriorityFeePerGas);
    return this;
  }

  /**
   * Sets the delay between receipt polls used by {@link SentTransaction#waitForReceipt()}. A
   * non-positive or {@code null} interval is deferred, not thrown.
   *
   * @param pollingInterval the delay between polls
   * @return this builder
   */
  public TxBuilder pollingInterval(final Duration pollingInterval) {
    if (pollingInterval == null || pollingInterval.isNegative() || pollingInterval.isZero()) {
      defer("polling interval must be positive, got: " + pollingInterval, null);
      return this;
    }
    this.pollingInterval = pollingInterval;
    return this;
  }

  /**
   * Sets how long {@link SentTransaction#waitForReceipt()} waits in total for a receipt before
   * failing with a {@link PaladinTimeoutException}. Overridable per call via {@link
   * SentTransaction#waitForReceipt(Duration)}. A non-positive or {@code null} timeout is deferred,
   * not thrown.
   *
   * @param receiptTimeout the total receipt wait
   * @return this builder
   */
  public TxBuilder receiptTimeout(final Duration receiptTimeout) {
    if (receiptTimeout == null || receiptTimeout.isNegative() || receiptTimeout.isZero()) {
      defer("receipt timeout must be positive, got: " + receiptTimeout, null);
      return this;
    }
    this.receiptTimeout = receiptTimeout;
    return this;
  }

  // ---------------------------------------------------------------------------------------------
  // Terminals
  // ---------------------------------------------------------------------------------------------

  /**
   * Validates the accumulated definition and assembles the immutable submission body.
   *
   * <p>This is where deferred errors surface. The builder is left untouched and may be reused or
   * further modified.
   *
   * @return the assembled transaction body
   * @throws PaladinInvalidTransactionException if a chaining call failed earlier, or the definition
   *     is structurally invalid (no type, no signing identity, a private transaction without a
   *     domain, a function invoke without a target, or a deploy whose bytecode does not match its
   *     type), or a privacy group is selected (use {@link #buildPrivacyGroup()} instead)
   */
  public TransactionInput build() {
    if (deferredError != null) {
      throw deferredError;
    }
    if (group != null) {
      throw new PaladinInvalidTransactionException(
          "use buildPrivacyGroup() for a privacy-group transaction");
    }
    validate(true);
    return transactionInput();
  }

  private TransactionInput transactionInput() {
    return TransactionInput.builder()
        .idempotencyKey(idempotencyKey)
        .type(type)
        .domain(domain)
        .function(function)
        .abiReference(abiReference)
        .from(from)
        .to(to)
        .data(data)
        .gas(gas)
        .value(value)
        .maxPriorityFeePerGas(maxPriorityFeePerGas)
        .maxFeePerGas(maxFeePerGas)
        .dependsOn(dependsOn)
        .abi(abi)
        .bytecode(bytecode)
        .build();
  }

  /**
   * Builds and submits the transaction, without waiting for it to be mined.
   *
   * <p>Returns immediately with a handle on the in-flight submission — matching Go's {@code
   * TxBuilder.Send()} and TypeScript's {@code sendTransaction()}, neither of which waits. Chain
   * {@link SentTransaction#waitForReceipt()} onto the result to wait for the receipt.
   *
   * <p>This never throws: a chain that recorded a deferred error, or a definition that fails
   * validation, produces a handle whose futures all fail with that {@link
   * PaladinInvalidTransactionException}.
   *
   * @return a handle on the submitted transaction, carrying the polling settings in force at this
   *     point
   */
  public SentTransaction send() {
    CompletableFuture<UUID> id;
    try {
      id =
          group == null
              ? ptx.sendTransaction(build())
              : privacyGroups.sendTransaction(buildPrivacyGroup());
    } catch (final PaladinInvalidTransactionException e) {
      id = CompletableFuture.failedFuture(e);
    }
    return new SentTransaction(ptx, id, pollingInterval, receiptTimeout);
  }

  /**
   * Builds the privacy-group submission body, resolving the function against the inline ABI.
   * Existing gas, value, and fee setters configure the nested base-ledger submission options. ABI
   * references and transaction dependencies are unsupported by the privacy-group RPC.
   *
   * @return the privacy-group transaction body
   * @throws PaladinInvalidTransactionException if the definition or a previous setter is invalid
   */
  public PrivacyGroupEVMTXInput buildPrivacyGroup() {
    return privacyGroupInput(true);
  }

  private PrivacyGroupEVMTXInput privacyGroupInput(final boolean requireFrom) {
    validateGroup(requireFrom);
    return PrivacyGroupEVMTXInput.builder(domain, group)
        .idempotencyKey(idempotencyKey)
        .from(from)
        .to(to)
        .input(data)
        .function(resolveGroupFunction())
        .bytecode(bytecode)
        .publicTxOptions(
            PublicTxOptions.builder()
                .gas(gas)
                .value(value)
                .maxFeePerGas(maxFeePerGas)
                .maxPriorityFeePerGas(maxPriorityFeePerGas)
                .build())
        .build();
  }

  /**
   * Executes a read-only call through the transaction or privacy-group namespace. The signing
   * identity is optional for calls. Uses {@link #block(String)} and {@link #dataFormat(String)}
   * when supplied.
   *
   * @return the decoded JSON result; validation and RPC errors complete the future exceptionally
   */
  public CompletableFuture<JsonNode> call() {
    try {
      if (group != null) {
        return privacyGroups.call(
            PrivacyGroupEVMCall.builder(privacyGroupInput(false))
                .block(block)
                .dataFormat(dataFormat)
                .build());
      }
      if (deferredError != null) {
        throw deferredError;
      }
      validate(false);
      return ptx.call(
          TransactionCall.builder(transactionInput()).block(block).dataFormat(dataFormat).build());
    } catch (final PaladinInvalidTransactionException e) {
      return CompletableFuture.failedFuture(e);
    }
  }

  private void validateGroup(final boolean requireFrom) {
    if (deferredError != null) {
      throw deferredError;
    }
    if (group == null) {
      throw new PaladinInvalidTransactionException("a privacy group is required");
    }
    if (type == TransactionType.PUBLIC) {
      throw new PaladinInvalidTransactionException("privacy-group transactions must be private");
    }
    if (isBlank(domain)) {
      throw new PaladinInvalidTransactionException(
          "a domain is required for privacy-group transactions");
    }
    if (requireFrom && isBlank(from)) {
      throw new PaladinInvalidTransactionException(
          "a signing identity is required: call from(...)");
    }
    if (abiReference != null || !dependsOn.isEmpty()) {
      throw new PaladinInvalidTransactionException(
          "privacy-group transactions do not support ABI references or dependencies");
    }
    if (isBlank(function)) {
      if (to != null) {
        throw new PaladinInvalidTransactionException(
            "a function is required when a target address is set");
      }
      if (bytecode == null || bytecode.isEmpty()) {
        throw new PaladinInvalidTransactionException(
            "bytecode is required for a privacy-group deploy");
      }
    } else if (to == null) {
      throw new PaladinInvalidTransactionException(
          "a target address is required to invoke a function");
    }
  }

  private AbiEntry resolveGroupFunction() {
    if (isBlank(function)) {
      return abi.stream()
          .filter(entry -> entry.type() == EntryType.CONSTRUCTOR)
          .findFirst()
          .orElse(abi.isEmpty() ? null : AbiEntry.constructor().build());
    }
    final List<AbiEntry> matches =
        abi.stream()
            .filter(entry -> entry.type() == EntryType.FUNCTION)
            .filter(entry -> function.equals(entry.name()) || function.equals(signature(entry)))
            .toList();
    if (matches.size() != 1) {
      throw new PaladinInvalidTransactionException(
          "function '"
              + function
              + "' must match exactly one inline ABI entry (use a full signature for overloads)");
    }
    return matches.getFirst();
  }

  private static String signature(final AbiEntry entry) {
    return entry.name()
        + "("
        + entry.inputs().stream().map(TxBuilder::parameterType).collect(Collectors.joining(","))
        + ")";
  }

  private static String parameterType(final AbiParameter parameter) {
    final String type = parameter.type();
    if (type.startsWith("tuple")) {
      return "("
          + parameter.components().stream()
              .map(TxBuilder::parameterType)
              .collect(Collectors.joining(","))
          + ")"
          + type.substring(5);
    }
    // Canonical ABI signatures expand Solidity's integer aliases, including array types.
    return type.replaceFirst("^uint(?=\\[|$)", "uint256").replaceFirst("^int(?=\\[|$)", "int256");
  }

  // ---------------------------------------------------------------------------------------------
  // Internals
  // ---------------------------------------------------------------------------------------------

  /** Records a deferred error; the first one wins, matching the Go builder. */
  private void defer(final String message, final Throwable cause) {
    if (deferredError == null) {
      deferredError =
          cause == null
              ? new PaladinInvalidTransactionException(message)
              : new PaladinInvalidTransactionException(message, cause);
    }
  }

  /**
   * Mirrors Go's {@code txBuilder.validateForSend}, plus the signing identity Go checks on send.
   */
  private void validate(final boolean requireFrom) {
    if (type == null) {
      throw new PaladinInvalidTransactionException(
          "transaction type is required: call publicTx(), privateTx() or type(...)");
    }
    if (requireFrom && isBlank(from)) {
      throw new PaladinInvalidTransactionException(
          "a signing identity is required: call from(...)");
    }
    if (type == TransactionType.PRIVATE && isBlank(domain)) {
      throw new PaladinInvalidTransactionException(
          "a domain is required for private transactions: call domain(...)");
    }
    if (isBlank(function)) {
      validateDeploy();
    } else if (to == null) {
      throw new PaladinInvalidTransactionException(
          "a target address is required to invoke function '" + function + "': call to(...)");
    }
  }

  /** Checks the constructor/deploy shape, reached only when no function is set. */
  private void validateDeploy() {
    if (to != null) {
      throw new PaladinInvalidTransactionException(
          "a function is required when a target address is set: call function(...)");
    }
    if (type == TransactionType.PRIVATE && bytecode != null) {
      throw new PaladinInvalidTransactionException(
          "bytecode cannot be supplied for a private deploy; the domain provides it");
    }
    if (type == TransactionType.PUBLIC && (bytecode == null || bytecode.isEmpty())) {
      throw new PaladinInvalidTransactionException(
          "bytecode is required for a public deploy: call bytecode(...)");
    }
  }

  private static boolean isBlank(final String s) {
    return s == null || s.isBlank();
  }
}
