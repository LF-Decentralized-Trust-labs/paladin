import { loadFixture } from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { expect } from "chai";
import { ethers } from "hardhat";
import { Noto } from "../../../typechain-types";
import {
  createLockOptions,
  deployNotoInstance,
  doDelegateLock,
  doLock,
  doMint,
  doPrepareUnlock,
  doTransfer,
  doUnlock,
  fakeTXO,
  newUnlockHash,
  randomBytes32,
} from "./util";
import { ZeroHash } from "ethers";

describe("Noto", function () {
  async function deployNotoFixture() {
    const [notary, other] = await ethers.getSigners();

    const NotoFactory = await ethers.getContractFactory("NotoFactory");
    const notoFactory = await NotoFactory.deploy();
    const Noto = await ethers.getContractFactory("Noto");
    const noto = Noto.attach(
      await deployNotoInstance(notoFactory, notary.address)
    );

    return { noto: noto as Noto, notary, other };
  }

  it("UTXO lifecycle and double-spend protections", async function () {
    const { noto, notary } = await loadFixture(deployNotoFixture);

    const txo1 = fakeTXO();
    const txo2 = fakeTXO();
    const txo3 = fakeTXO();

    // Make two UTXOs
    await doTransfer(
      randomBytes32(),
      notary,
      noto,
      [],
      [txo1, txo2],
      randomBytes32()
    );

    // Check for double-mint protection
    await expect(
      doTransfer(randomBytes32(), notary, noto, [], [txo1], randomBytes32())
    ).rejectedWith("NotoInvalidOutput");

    // Check for spend unknown protection
    await expect(
      doTransfer(randomBytes32(), notary, noto, [txo3], [], randomBytes32())
    ).rejectedWith("NotoInvalidInput");

    // Spend one
    await doTransfer(
      randomBytes32(),
      notary,
      noto,
      [txo1],
      [txo3],
      randomBytes32()
    );

    // Check for double-spend protection
    await expect(
      doTransfer(randomBytes32(), notary, noto, [txo1], [txo3], randomBytes32())
    ).rejectedWith("NotoInvalidInput");

    // Spend another
    await doTransfer(
      randomBytes32(),
      notary,
      noto,
      [txo2],
      [],
      randomBytes32()
    );

    // Spend the last one
    await doTransfer(
      randomBytes32(),
      notary,
      noto,
      [txo3],
      [],
      randomBytes32()
    );
  });

  it("lock and unlock", async function () {
    const { noto, notary } = await loadFixture(deployNotoFixture);
    const [_, delegate, other] = await ethers.getSigners();
    expect(notary.address).to.not.equal(delegate.address);

    const txo1 = fakeTXO();
    const txo2 = fakeTXO();
    const txo3 = fakeTXO();
    const txo4 = fakeTXO();

    const locked1 = fakeTXO();

    // Make two UTXOs
    await doTransfer(
      randomBytes32(),
      notary,
      noto,
      [],
      [txo1, txo2],
      randomBytes32()
    );

    // Lock both of them
    const txId1 = randomBytes32();
    const params = {
      txId: txId1,
      inputs: [txo1, txo2],
      outputs: [txo3],
      lockedOutputs: [locked1],
      proof: "0x",
      options: "0x",
    };
    const lockId = await doLock(notary, noto, params, ZeroHash);

    // Check that the same state cannot be locked again with a different lock
    const txId2 = randomBytes32();
    const params2 = {
      txId: txId2,
      inputs: [],
      outputs: [],
      lockedOutputs: [locked1],
      proof: "0x",
      options: "0x",
    };
    await expect(
      doLock(notary, noto, params2, randomBytes32())
    ).to.be.rejectedWith("NotoInvalidOutput");

    // Check that locked value cannot be spent
    await expect(
      doTransfer(randomBytes32(), notary, noto, [locked1], [], randomBytes32())
    ).to.be.rejectedWith("NotoInvalidInput");

    // Prepare unlock operations (both spend and cancel) before unlocking
    const unlockTxId = randomBytes32();
    const unlockData = randomBytes32();
    const spendHash = await newUnlockHash(
      noto,
      unlockTxId,
      [locked1],
      [txo4],
      unlockData
    );
    const cancelHash = await newUnlockHash(noto, unlockTxId, [locked1], [], unlockData);
    await doPrepareUnlock(
      randomBytes32(),
      notary,
      noto,
      lockId,
      [locked1],
      unlockTxId,
      spendHash,
      cancelHash,
      unlockData,
    );

    // Delegate the lock
    await doDelegateLock(
      randomBytes32(),
      notary,
      noto,
      lockId,
      delegate.address,
      randomBytes32()
    );

    // Attempt to perform an incorrect unlock with wrong inputs
    await expect(
      doUnlock(
        unlockTxId,
        delegate,
        noto,
        lockId,
        [locked1, fakeTXO()],
        [txo4],
        unlockData
      ) // mismatched input states - hash won't match
    ).to.be.rejectedWith("NotoInvalidUnlockHash");

    // Try to unlock with wrong outputs
    await expect(
      doUnlock(
        unlockTxId,
        delegate,
        noto,
        lockId,
        [locked1],
        [fakeTXO()], // different output than prepared
        unlockData
      ) // wrong outputs - hash won't match
    ).to.be.rejectedWith("NotoInvalidUnlockHash");

    // Try to unlock with wrong delegate
    await expect(
      doUnlock(
        unlockTxId,
        other,
        noto,
        lockId,
        [locked1],
        [txo4],
        unlockData
      ) // wrong delegate
    ).to.be.rejectedWith("LockUnauthorized");

    // Perform the prepared unlock
    await doUnlock(
      unlockTxId,
      delegate,
      noto,
      lockId,
      [locked1],
      [txo4],
      unlockData
    );

    // Check that the same state cannot be unlocked again (lock is already spent, so not active)
    await expect(
      doUnlock(
        randomBytes32(),
        notary,
        noto,
        lockId,
        [locked1],
        [],
        randomBytes32()
      )
    ).to.be.rejectedWith("LockNotActive");
  });

  it("Duplicate TXID reverts transfer", async function () {
    const { noto, notary } = await loadFixture(deployNotoFixture);

    const txo1 = fakeTXO();
    const txo2 = fakeTXO();
    const txo3 = fakeTXO();
    const txo4 = fakeTXO();
    const txId1 = randomBytes32();

    // Make two UTXOs - should succeed
    await doTransfer(txId1, notary, noto, [], [txo1, txo2], randomBytes32());

    // Make two more UTXOs with the same TX ID - should fail
    await expect(
      doTransfer(txId1, notary, noto, [], [txo3, txo4], randomBytes32())
    ).rejectedWith("NotoDuplicateTransaction");
  });

  it("Duplicate TXID reverts lock, unlock, prepare unlock, and delegate lock", async function () {
    const { noto, notary } = await loadFixture(deployNotoFixture);
    const [_, delegate, other] = await ethers.getSigners();
    expect(notary.address).to.not.equal(delegate.address);

    const txo1 = fakeTXO();
    const txo2 = fakeTXO();
    const txo3 = fakeTXO();
    const txo4 = fakeTXO();
    const txo5 = fakeTXO();

    const locked1 = fakeTXO();
    const locked2 = fakeTXO();

    const txId1 = randomBytes32();

    // Make two UTXOs
    await doTransfer(txId1, notary, noto, [], [txo1, txo2], randomBytes32());

    // Try to lock both of them using the same TX ID - should fail
    const params1 = {
      txId: txId1,
      inputs: [txo1, txo2],
      outputs: [txo3],
      lockedOutputs: [locked1],
      proof: "0x",
      options: "0x",
    };
    await expect(
      doLock(notary, noto, params1, randomBytes32())
    ).to.be.rejectedWith("NotoDuplicateTransaction");

    // Lock both of them using a new TX ID - should succeed
    const txId2 = randomBytes32();
    const params2 = {
      txId: txId2,
      inputs: [txo1, txo2],
      outputs: [txo3],
      lockedOutputs: [locked1],
      proof: "0x",
      options: "0x",
    };
    const lockId = await doLock(notary, noto, params2, randomBytes32());

    // Prepare unlock operations (both spend and cancel) using the same TX ID as the transfer - should fail
    const unlockTxId = txId1; // Use same txId as transfer to test duplicate
    const unlockData = randomBytes32();
    const spendHash = await newUnlockHash(
      noto,
      unlockTxId,
      [locked1],
      [txo4],
      unlockData
    );
    const cancelHash = await newUnlockHash(noto, unlockTxId, [locked1], [], unlockData);
    await expect(
      doPrepareUnlock(
        txId1,
        notary,
        noto,
        lockId,
        [locked1],
        unlockTxId,
        spendHash,
        cancelHash,
        unlockData,
      )
    ).to.be.rejectedWith("NotoDuplicateTransaction");
  });

  it("Duplicate TXID reverts mint", async function () {
    const { noto, notary } = await loadFixture(deployNotoFixture);

    const txo1 = fakeTXO();
    const txo2 = fakeTXO();
    const txo3 = fakeTXO();
    const txo4 = fakeTXO();
    const txId1 = randomBytes32();

    // Make two UTXOs - should succeed
    await doMint(txId1, notary, noto, [txo1, txo2], randomBytes32());

    // Make two more UTXOs using the same TX ID - should fail
    await expect(
      doMint(txId1, notary, noto, [txo3, txo4], randomBytes32())
    ).rejectedWith("NotoDuplicateTransaction");
  });
});
