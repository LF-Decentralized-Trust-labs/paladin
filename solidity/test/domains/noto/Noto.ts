import { loadFixture } from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { expect } from "chai";
import { ethers } from "hardhat";
import { Noto } from "../../../typechain-types";
import {
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
    const txo5 = fakeTXO();

    const locked1 = fakeTXO();
    const locked2 = fakeTXO();

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
    await doLock(
      randomBytes32(),
      notary,
      noto,
      [txo1, txo2],
      [txo3],
      [locked1],
      randomBytes32()
    );

    // Check that the same state cannot be locked again with the same lock
    await expect(
      doLock(randomBytes32(), notary, noto, [], [], [locked1], randomBytes32())
    ).to.be.rejectedWith("NotoInvalidOutput");

    // Check that locked value cannot be spent
    await expect(
      doTransfer(randomBytes32(), notary, noto, [locked1], [], randomBytes32())
    ).to.be.rejectedWith("NotoInvalidInput");

    // Unlock the UTXO
    await doUnlock(
      randomBytes32(),
      notary,
      noto,
      [locked1],
      [locked2],
      [txo4],
      randomBytes32()
    );

    // Check that the same state cannot be unlocked again
    await expect(
      doUnlock(
        randomBytes32(),
        notary,
        noto,
        [locked1],
        [],
        [],
        randomBytes32()
      )
    ).to.be.rejectedWith("NotoInvalidInput");

    // Prepare an unlock operation
    const unlockData = randomBytes32();
    const unlockHash = await newUnlockHash(
      noto,
      [locked2],
      [],
      [txo5],
      unlockData
    );
    await doPrepareUnlock(notary, noto, [locked2], unlockHash, unlockData);

    // Delegate the unlock
    await doDelegateLock(
      randomBytes32(),
      notary,
      noto,
      unlockHash,
      delegate.address,
      randomBytes32()
    );

    // Attempt to perform an incorrect unlock
    await expect(
      doUnlock(
        randomBytes32(),
        delegate,
        noto,
        [locked1, locked2],
        [],
        [txo5],
        unlockData
      ) // mismatched input states
    ).to.be.rejectedWith("NotoInvalidInput");
    await expect(
      doUnlock(
        randomBytes32(),
        delegate,
        noto,
        [locked2],
        [],
        [txo5],
        randomBytes32()
      ) // wrong data
    ).to.be.rejectedWith("NotoInvalidUnlockHash");
    await expect(
      doUnlock(randomBytes32(), other, noto, [locked2], [], [txo5], unlockData) // wrong delegate
    ).to.be.rejectedWith("NotoInvalidDelegate");

    // Perform the prepared unlock
    await doUnlock(
      randomBytes32(),
      delegate,
      noto,
      [locked2],
      [],
      [txo5],
      unlockData
    );
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
    await expect(
      doLock(
        txId1,
        notary,
        noto,
        [txo1, txo2],
        [txo3],
        [locked1],
        randomBytes32()
      )
    ).to.be.rejectedWith("NotoDuplicateTransaction");

    // Lock both of them using a new TX ID - should succeed
    await doLock(
      randomBytes32(),
      notary,
      noto,
      [txo1, txo2],
      [txo3],
      [locked1],
      randomBytes32()
    );

    // Unlock the UTXO using the same TX ID as the transfer - should fail
    await expect(
      doUnlock(
        txId1,
        notary,
        noto,
        [locked1],
        [locked2],
        [txo4],
        randomBytes32()
      )
    ).to.be.rejectedWith("NotoDuplicateTransaction");

    // Prepare an unlock operation
    const unlockData = randomBytes32();
    const unlockHash = await newUnlockHash(
      noto,
      [locked2],
      [],
      [txo5],
      unlockData
    );

    // Delegate the lock using the same TX ID as the transfer - should fail
    await expect(
      doDelegateLock(
        txId1,
        notary,
        noto,
        unlockHash,
        delegate.address,
        randomBytes32()
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
