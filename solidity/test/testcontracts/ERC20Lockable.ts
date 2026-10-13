import { expect } from "chai";
import type { Signer } from "ethers";
import { ethers } from "hardhat";
import type { ERC20Lockable } from "../../typechain-types";
import {
  createTokenLock,
  encodeCreateLockInputs,
  ERC20_CREATE_LOCK_INPUTS_TYPE,
  ERC20_LOCK_CONTENT_TYPE,
} from "../helpers/erc20-lockable";

describe("ERC20Lockable", function () {
  let token: ERC20Lockable;
  let owner: Signer;
  let alice: Signer;
  let bob: Signer;
  let charlie: Signer;

  let aliceAddress: string;
  let bobAddress: string;
  let charlieAddress: string;

  beforeEach(async function () {
    [owner, alice, bob, charlie] = await ethers.getSigners();

    aliceAddress = await alice.getAddress();
    bobAddress = await bob.getAddress();
    charlieAddress = await charlie.getAddress();

    // Deploy ERC20Lockable token
    const ERC20LockableFactory = await ethers.getContractFactory(
      "ERC20Lockable",
    );
    token = (await ERC20LockableFactory.connect(owner).deploy(
      "LockableToken",
      "LOCK",
    )) as ERC20Lockable;

    // Mint tokens to alice
    await token.connect(owner).mint(aliceAddress, ethers.parseEther("1000"));
  });

  describe("Basic ERC20 functionality", function () {
    it("should have correct name and symbol", async function () {
      expect(await token.name()).to.equal("LockableToken");
      expect(await token.symbol()).to.equal("LOCK");
    });

    it("should allow normal transfers when no locks exist", async function () {
      await token.connect(alice).transfer(bobAddress, ethers.parseEther("100"));

      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("900"),
      );
      expect(await token.balanceOf(bobAddress)).to.equal(
        ethers.parseEther("100"),
      );
    });

    it("should show correct locked and unlocked balances", async function () {
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        ethers.parseEther("1000"),
      );
    });
  });

  describe("Lock creation and management", function () {
    it("should create a lock successfully", async function () {
      const amount = ethers.parseEther("200");

      const lockId = await createTokenLock(token, alice, amount, bobAddress);

      // Verify locked balance updated
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(amount);
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        ethers.parseEther("800"),
      );
      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("1000"),
      );

      // Verify we got a valid lock ID
      expect(lockId).to.be.a("string");
      expect(lockId).to.have.length.greaterThan(0);

      // Verify lock info
      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.owner).to.equal(aliceAddress);
      expect(lockInfo.spender).to.equal(aliceAddress);

      const content = await token.getLockContent(lockId);
      const expectedSelector = ethers.id(ERC20_LOCK_CONTENT_TYPE).slice(0, 10);
      expect(content.slice(0, 10)).to.equal(expectedSelector);
      const [inputs] = ethers.AbiCoder.defaultAbiCoder().decode(
        [ERC20_CREATE_LOCK_INPUTS_TYPE],
        "0x" + content.slice(10),
      );
      expect(inputs.amount).to.equal(amount);
      expect(inputs.recipient).to.equal(bobAddress);
    });

    it("should revert when trying to lock more than unlocked balance", async function () {
      const amount = ethers.parseEther("1500"); // More than alice's balance
      const createInputs = encodeCreateLockInputs(amount, bobAddress);

      await expect(
        token
          .connect(alice)
          .createLock(createInputs, ethers.ZeroHash, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "ERC20InsufficientBalance");
    });

    it("should prevent transfers when insufficient unlocked balance", async function () {
      const lockAmount = ethers.parseEther("800");

      await createTokenLock(token, alice, lockAmount, bobAddress);

      // Try to transfer more than unlocked balance (200 available, trying 300)
      await expect(
        token.connect(alice).transfer(bobAddress, ethers.parseEther("300")),
      ).to.be.revertedWithCustomError(token, "ERC20InsufficientBalance");

      // But should allow transfer within unlocked balance
      await token.connect(alice).transfer(bobAddress, ethers.parseEther("100"));
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        ethers.parseEther("100"),
      );
    });
  });

  describe("Lock spending", function () {
    let lockId: string;

    beforeEach(async function () {
      const amount = ethers.parseEther("300");
      lockId = await createTokenLock(token, alice, amount, bobAddress);
    });

    it("should spend lock successfully and delete it", async function () {
      const initialAliceBalance = await token.balanceOf(aliceAddress);
      const initialBobBalance = await token.balanceOf(bobAddress);

      await token.connect(alice).spendLock(lockId, "0x", "0x");

      // Verify token transfer occurred
      expect(await token.balanceOf(aliceAddress)).to.equal(
        initialAliceBalance - ethers.parseEther("300"),
      );
      expect(await token.balanceOf(bobAddress)).to.equal(
        initialBobBalance + ethers.parseEther("300"),
      );

      // Verify locked balance reduced
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        await token.balanceOf(aliceAddress),
      );

      // Verify lock is deleted
      await expect(token.getLock(lockId)).to.be.revertedWithCustomError(
        token,
        "LockNotActive",
      );
    });

    it("should revert if non-spender tries to spend", async function () {
      await expect(
        token.connect(bob).spendLock(lockId, "0x", "0x"),
      ).to.be.revertedWithCustomError(token, "LockUnauthorized");
    });
  });

  describe("Lock cancellation", function () {
    let lockId: string;

    beforeEach(async function () {
      const amount = ethers.parseEther("300");
      lockId = await createTokenLock(token, alice, amount, bobAddress);
    });

    it("should cancel lock successfully and delete it", async function () {
      const initialAliceBalance = await token.balanceOf(aliceAddress);

      await token.connect(alice).cancelLock(lockId, "0x", "0x");

      // Verify no token transfer occurred
      expect(await token.balanceOf(aliceAddress)).to.equal(initialAliceBalance);

      // Verify locked balance reduced (tokens unlocked)
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        initialAliceBalance,
      );

      // Verify lock is deleted
      await expect(token.getLock(lockId)).to.be.revertedWithCustomError(
        token,
        "LockNotActive",
      );
    });

    it("should revert if non-spender tries to cancel", async function () {
      await expect(
        token.connect(bob).cancelLock(lockId, "0x", "0x"),
      ).to.be.revertedWithCustomError(token, "LockUnauthorized");
    });
  });

  describe("Lock delegation", function () {
    let lockId: string;

    beforeEach(async function () {
      const amount = ethers.parseEther("300");
      lockId = await createTokenLock(token, alice, amount, bobAddress);
    });

    it("should delegate lock successfully", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");

      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.spender).to.equal(charlieAddress);
      expect(lockInfo.owner).to.equal(aliceAddress); // Owner unchanged

      // Verify charlie can now spend the lock
      await token.connect(charlie).spendLock(lockId, "0x", "0x");

      // Verify tokens transferred from alice to bob
      expect(await token.balanceOf(bobAddress)).to.equal(
        ethers.parseEther("300"),
      );
    });

    it("should revert if alice tries to spend after delegation", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");

      await expect(
        token.connect(alice).spendLock(lockId, "0x", "0x"),
      ).to.be.revertedWithCustomError(token, "LockUnauthorized");
    });

    it("should revert if trying to update lock after delegation", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");

      await expect(
        token
          .connect(charlie)
          .updateLock(
            lockId,
            "0x",
            ethers.keccak256(ethers.toUtf8Bytes("new spend hash")),
            ethers.ZeroHash,
            "0x",
          ),
      ).to.be.revertedWithCustomError(token, "LockImmutable");
    });
  });
});
