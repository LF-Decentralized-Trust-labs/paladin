# Bond Issuance

The code for this tutorial can be found in [example/bond](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/bond).

This shows how to leverage the [Noto](../../architecture/noto/) and [Pente](../../architecture/pente/) domains together in order to build a bond issuance process, illustrating multiple aspects of Paladin's privacy capabilities.

![Bond issuance](../../images/paladin_bond.png)

## Running the example

Follow the [Getting Started](../../getting-started/installation/) instructions to set up a Paladin environment, and
then follow the example [README](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/bond/README.md)
to run the code.

## Explanation

Below is a walkthrough of each step in the example, with an explanation of what it does.

### Cash setup

#### Deploy cash token

```typescript
const notoFactory = new NotoFactory(paladin1, "noto");
const notoCash = await notoFactory.newNoto(cashIssuer, {
  notary: cashIssuer,
  restrictMinting: true,
});
```

This creates a new instance of the Noto domain, which will translate to a new cloned contract
on the base ledger, with a new unique address. This Noto token will be used to represent
tokenized cash.

The token will be notarized by the cash issuer party, meaning that all transactions will be
sent to this party for endorsement. The Noto domain code at the notary will automatically
verify that each transaction is valid - no custom policies will be applied.

Minting is restricted to be requested only by the notary.

#### Issue cash

```typescript
await notoCash.mint(cashIssuer, {
  to: investor,
  amount: 100000,
  data: "0x",
});
```

The cash issuer mints cash to the investor party. As the notary of the cash token, they are
allowed to do this.

### Bond setup

#### Create issuer+custodian private group

```typescript
const penteFactory = new PenteFactory(paladin1, "pente");
const issuerCustodianGroup = await penteFactory.newPrivacyGroup(bondIssuer, {
  group: {
    salt: newGroupSalt(),
    members: [bondIssuer, bondCustodian],
  },
  evmVersion: "shanghai",
  endorsementType: "group_scoped_identities",
  externalCallsEnabled: true,
});
```

This creates a new instance of the Pente domain, which will be a private EVM group shared by the
bond issuer and bond custodian. Each transaction proposed in this private EVM will need to be
endorsed by both participants, and then the new state of the private EVM can be hashed and
recorded on the base ledger contract.

As will be shown in future steps, any EVM logic can be deployed into this private EVM group.

#### Create public bond tracker

```typescript
await paladin1.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: bondTrackerPublicJson.abi,
  bytecode: bondTrackerPublicJson.bytecode,
  function: "",
  from: bondIssuer.lookup,
  data: {
    owner: issuerCustodianGroup.address,
    issueDate_: issueDate,
    maturityDate_: maturityDate,
    currencyToken_: notoCash.address,
    faceValue_: 1,
  },
});
```

This sends a public EVM transaction to deploy the [public bond tracker](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/shared/BondTrackerPublic.sol).
This is equivalent to performing a deploy directly on the base ledger, without any special
handling for privacy.

The public bond tracker is used to advertise public information about our bond, such as its
face value, issue date, and maturity date. It will also be updated with publically-visible
events throughout the bond's lifetime.

#### Create private bond tracker

```typescript
await newBondTracker(issuerCustodianGroup, bondIssuer, {
  name: "BOND",
  symbol: "BOND",
  custodian: await bondCustodian.address(),
  publicTracker: bondTrackerPublicAddress,
});
```

The [private bond tracker](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/private/BondTracker.sol)
is an ERC-20 token, and implements the [INotoHooks](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/private/interfaces/INotoHooks.sol) interface.

Noto supports using a Pente private smart contract to define "hooks" which are executed inline with every mint/transfer.
This provides a flexible (and EVM-native) means of writing custom policies that are enforced by the notary. In this case,
the notary tracks the bond value as an ERC-20. Every mint and transfer of the Noto token will be reflected on this private
ERC-20, and anything that causes the ERC-20 to revert will cause the Noto operation to revert. This private tracker is only
visible to the bond issuer and custodian, but will be atomically linked to the Noto token in the next step.

#### Deploy bond token

```typescript
const notoBond = await notoFactory.newNoto(bondIssuer, {
  notary: bondCustodian,
  hooks: {
    privateGroup: issuerCustodianGroup.group,
    publicAddress: issuerCustodianGroup.address,
    privateAddress: bondTracker.address,
  },
  restrictMinting: false,
});
```

Now that the public and private tracking contracts have been deployed, the actual Noto token for the bond can be created.
The "hooks" configuration points it to the private hooks contract that was deployed in the previous step.

For this token, "restrictMinting" is disabled, because the hooks can enforce more flexible rules on both mint and transfer.

#### Create factory for atomic transactions

```typescript
await paladin1.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: atomFactoryJson.abi,
  bytecode: atomFactoryJson.bytecode,
  function: "",
  from: bondIssuer.lookup,
  data: {},
});
```

Many programming patterns in Paladin will require a contract on the shared ledger that
can prepare and execute atomic transactions. This is provided by the
[Atom and AtomFactory](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/shared/Atom.sol) contracts.

At least one instance of `AtomFactory` must be deployed to run this example. Once in place,
note that this same factory contract can be reused for atomic transactions of any composition.

### Bond issuance

#### Issue bond to custodian

```typescript
await notoBond.mint(bondIssuer, {
  to: bondCustodian,
  amount: 1000,
  data: "0x",
});
```

This issues the bond to the bond custodian.

The Noto "mint" request will be prepared and encoded within a call to the "onMint" hook in the private bond tracker
contract. The logic in that contract will validate that the mint is allowed, and then will trigger two external calls
on the base ledger: 1) to perform the Noto mint, and 2) to notify the public bond tracker that issuance has started.

#### Begin distribution of bond

```typescript
await bondTracker.using(paladin2).beginDistribution(bondCustodian, {
  discountPrice: 1,
  minimumDenomination: 1,
});
const investorList = await bondTracker.investorList(bondIssuer);
await investorList
  .using(paladin2)
  .addInvestor(bondCustodian, { addr: await investor.address() });
```

This allows the bond custodian to begin distributing the bond to potential investors. Each investor must be added
to the allow list before they will be allowed to subscribe to the bond.

Both the bond tracker and the investor registry are private contracts, visible only within the privacy group
between the issuer and custodian.

### Bond subscription

#### Create investor+custodian private group

```typescript
const investorCustodianGroup = await penteFactory
  .using(paladin3)
  .newPrivacyGroup(investor, {
    group: {
      salt: newGroupSalt(),
      members: [investor, bondCustodian],
    },
    evmVersion: "shanghai",
    endorsementType: "group_scoped_identities",
    externalCallsEnabled: true,
  });
```

This creates another instance of the Pente domain, scoped to only the investor and the custodian.

#### Create private bond subscription

```typescript
const bondSubscription = await newBondSubscription(
  investorCustodianGroup,
  investor,
  {
    bondAddress_: notoBond.address,
    units_: 100,
    custodian_: await bondCustodian.address(),
    atomFactory_: atomFactoryAddress,
  }
);
```

An investor may request to subscribe to the bond by creating a [private subscription contract](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/private/BondSubscription.sol)
in their private EVM with the bond custodian.

#### Prepare tokens for exchange

```typescript
const paymentTransfer = await notoCash
  .using(paladin3)
  .prepareTransfer(investor, {
    to: bondCustodian,
    amount: 100,
    data: "0x",
  });

const bondTransfer1 = await notoBond
  .using(paladin2)
  .prepareTransfer(bondCustodian, {
    to: investor,
    amount: 100,
    data: "0x",
  });
txID = await paladin2.prepareTransaction(bondTransfer1.transaction);
const bondTransfer2 = await paladin2.pollForPreparedTransaction(txID, 10000);
```

The investor prepares (but does not submit) a cash payment. This results in a public transaction
containing prepared UTXO states. The transaction can be delegated to another party or contract to
allow them to execute the payment transfer.

The bond custodian prepares a similar transaction for the bond transfer. Because the bond token
uses Pente hooks, the result of the first "prepare" is a private Pente transaction. This can
be prepared again to receive a public transaction that wraps both the Pente hook transition and
the Noto bond token transfer.

#### Share the prepared transactions with the private contract

```typescript
await bondSubscription.using(paladin3).preparePayment(investor, {
  to: paymentTransfer.transaction.to,
  encodedCall: paymentTransfer.metadata?.transferWithApproval?.encodedCall,
});

await bondSubscription.using(paladin2).prepareBond(bondCustodian, {
  to: bondTransfer2.transaction.to,
  encodedCall: bondTransfer2.metadata.transitionWithApproval.encodedCall,
});
```

The `preparePayment` and `prepareBond` methods on the bond subscription contract allow the
respective parties to encode their prepared transactions, in preparation for triggering an
atomic DvP (delivery vs. payment).

#### Prepare the atomic transaction for the swap

```typescript
await bondSubscription.using(paladin2).distribute(bondCustodian);
```

When both parties have prepared their individual transactions, they can be combined into a
single base ledger transaction. The `distribute()` method below is a private method on
the `BondSubscription` contract, but it triggers creation of a new `Atom` contract on the
base ledger which contains the encoded transactions prepared above.

Once an `Atom` is deployed, it can be used to execute all or none of the transactions it
contains. It can never be changed, executed partially, or executed more than once.

#### Approve delegation via the private contract

```typescript
await notoCash.using(paladin3).approveTransfer(investor, {
  inputs: encodeStates(paymentTransfer.states.spent ?? []),
  outputs: encodeStates(paymentTransfer.states.confirmed ?? []),
  data: paymentTransfer.metadata.approvalParams.data,
  delegate: atomAddress,
});

await issuerCustodianGroup.approveTransition(
  bondCustodianUnqualified,
  {
    txId: newTransactionId(),
    transitionHash: bondTransfer2.metadata.approvalParams.transitionHash,
    signatures: bondTransfer2.metadata.approvalParams.signatures,
    delegate: atomAddress,
  }
);
```

Once the `Atom` is deployed, it must be designated as the approved delegate for both
the payment transfer and the bond transfer. Because this binds a specific set of atomic
operations to a unique contract address, both parties can be assured that by approving
this address as a delegate, the only transaction that can take place is the agreed swap.

In the case of the payment, we use the `approveTransfer` method of Noto. For the bond,
which uses Pente custom logic to wrap the Noto token, we use the `approveTransition` method
of Pente.

#### Distribute the bond units by performing swap

```typescript
await paladin2.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: atomJson.abi,
  function: "execute",
  from: bondCustodian.lookup,
  to: atomAddress,
  data: {},
});
```

Finally, the custodian executes the `Atom` to trigger the exchange of the bond and payment.

This will trigger the previously-prepared transactions for the cash
transfer and the bond transfer, and it will also trigger an external call to the public
bond tracker to decrease the advertised available supply of the bond.

## Conclusion

This scenario shows how to work with the following concepts:

- Basic Noto tokens
- Noto tokens with custom hooks via Pente
- Multiple Pente privacy groups used for sharing private data
- Pente private smart contracts that trigger external calls to contracts on the base ledger

By using these features together, it's possible to build a robust issuance process that
tracks all state on the base EVM ledger, while still keeping all private data scoped to
only the proper parties.
