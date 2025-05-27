# Cash Tokens based on Zeto

The code for this tutorial can be found in [example/zeto](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/zeto).

This shows how to leverage the [Zeto](../../architecture/zeto/) in order to build a cash payment solution, for instance wholesale CBDC or a payment rail with commercial bank money, with privacy, illustrating multiple aspects of Paladin's privacy capabilities.

## Running the example

Follow the [Getting Started](../../getting-started/installation/) instructions to set up a Paladin environment, and
then follow the example [README](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/zeto/README.md)
to run the code.

## Scenario #1: cash solution with private minting

In this scenario, the Zeto tokens are directly minted by the authority in the Zeto contract, making the mint amounts private. This also means the total supply of the Zeto tokens is unknown to the participants. Only the authority performing the minting operations is aware of the total supply.

Below is a walkthrough of each step in the example, with an explanation of what it does.

### Create CBDC token

```typescript
const zetoFactory = new ZetoFactory(paladin3, 'zeto');
const zetoCBDC = await zetoFactory.newZeto(cbdcIssuer, {
  tokenName: 'Zeto_AnonNullifier',
});
```

This creates a new instance of the Zeto domain, using the [Zeto_AnonNullifier](https://github.com/hyperledger-labs/zeto/tree/main?tab=readme-ov-file#zeto_anonnullifier) contract. This results in a new cloned contract on the base ledger, with a new unique address. This Zeto token contract will be used to represent
tokenized cash/CBDC.

The token will be minted by the central bank/CBDC issuer party. Minting is restricted to be requested only by the central bank, the
deployer account of the contract.

### Issue cash

```typescript
let receipt = await zetoCBDC.mint(cbdcIssuer, {
  mints: [
    {
      to: bank1,
      amount: 100000,
      data: "0x",
    },
    {
      to: bank2,
      amount: 100000,
      data: "0x",
    },
  ],
});
```

The cash issuer mints cash to the commercial banks, `bank1` and `bank2`.

### Bank1 transfers tokens to bank2 as payment

```typescript
receipt = await zetoCBDC.using(paladin1).transfer(bank1, {
  transfers: [
    {
      to: bank2,
      amount: 1000,
      data: "0x",
    },
  ],
});
```

Bank1 can call the `transfer` function to transfer zeto tokens to multiple parties, up to 10. Note that the identity `bank1` exists on the `paladin1` instance,
therefore it must use that instance to send the transfer transction (`.using(paladin1)`).

## Scenario #2: cash solution with public minting

This scenario supports the requirement to make the total supply of the cash tokens public. This is achieved by making the authority perform the minting operations in an ERC20 contract. The participants can then exchange their ERC20 balances for Zeto tokens, by calling `deposit`, and exchange back to their ERC20 balances by calling `withdraw`.

Below is a walkthrough of each step in the example, with an explanation of what it does.

### Create CBDC token

```typescript
const zetoFactory = new ZetoFactory(paladin3, 'zeto');
const zetoCBDC = await zetoFactory.newZeto(cbdcIssuer, {
  tokenName: 'Zeto_AnonNullifier',
});
```

This creates a new instance of the Zeto domain, using the [Zeto_AnonNullifier](https://github.com/hyperledger-labs/zeto/tree/main?tab=readme-ov-file#zeto_anonnullifier) contract. This results in a new cloned contract on the base ledger, with a new unique address. This Zeto token contract will be used to represent
tokenized cash/CBDC.

### Create public supply token (ERC20)

```typescript
const erc20Address = await deployERC20(paladin3, cbdcIssuer);
```

This deploys the ERC20 token which will be used by the authority to regulate the CBDC supply, with transparency to the paricipants.

### Configure the Zeto token contract to accept deposits and withdraws from the ERC20

```typescript
const result2 = await zetoCBDC.setERC20(cbdcIssuer, {
  _erc20: erc20Address as string,
});
```

When the `deposit` function is called on the Zeto contract, this ERC20 contract will be called to draw the requested funds from the depositor's account. Conversely, when the `withdraw` function is called, this ERC20 contract will be called to transfer back the ERC20 balance to the withdrawer's account.

### Mint ERC20 tokens to publicly regulate CBDC supplies

```typescript
await mintERC20(paladin3, cbdcIssuer, bank1, erc20Address!, 100000);
```

Because the ERC20 implementation provides full transparency of the token operations, minting in the ERC20 allows all blockchain network participants to be aware of the overall supply of the CBDC tokens.

### Banks exchange ERC20 balances for Zeto tokens - deposit

```typescript
const result4 = await zetoCBDC.using(paladin1).deposit(bank1, {
  amount: 10000,
});
```

After having been minted ERC20 balances, a partcipant like `bank1` can call `deposit` on the Paladin Zeto domain to exchange for Zeto tokens. Behind the scenes, the ERC20 balance is transferred to the Zeto contract which will hold until `withdraw` is called later.

### Bank1 transfers tokens to bank2 as payment

```typescript
receipt = await zetoCBDC.using(paladin1).transfer(bank1, {
  transfers: [
    {
      to: bank2,
      amount: 1000,
      data: "0x",
    },
  ],
});
```

Bank1 can call the `transfer` function to transfer zeto tokens to multiple parties, up to 10. Note that the identity `bank1` exists on the `paladin1` instance,
therefore it must use that instance to send the transfer transction (`.using(paladin1)`).

### Bank1 exchanges Zeto tokens for ERC20 balances - withdraw

```typescript
const result5 = await zetoCBDC.using(paladin1).withdraw(bank1, {
  amount: 1000,
});
```

A participant like `bank1` who has unspent Zeto tokens can call `withdraw` on the Paladin Zeto domain to exchange them for ERC20 balances. Behind the scenes, the requested amount are "burnt" in the Zeto contract, and the corresponding ERC20 amount are released by the Zeto contract, by transferring to the requesting account.

## Next Steps

Next, discover how **Notarized Tokens** and **Privacy Groups** seamlessly integrate to enable a robust bond issuance workflow that balances private collaboration with public transparency.

[Continue to the Bond Issuance Tutorial →](./bond-issuance.md)
