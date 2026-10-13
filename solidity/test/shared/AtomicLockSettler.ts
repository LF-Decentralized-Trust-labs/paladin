import { expect } from "chai";
import type { Signer } from "ethers";
import { ethers } from "hardhat";
import type { AtomicLockSettler, AtomicLockSettlerFactory, ERC20Lockable } from "../../typechain-types";
import { deployUpgradeableContract } from "../helpers/deploy-upgradeable";
import { createTokenLock } from "../helpers/erc20-lockable";

describe("AtomicLockSettler", function () {
  let lockSettlerFactory: AtomicLockSettlerFactory;

  before(async function () {
    lockSettlerFactory = (await deployUpgradeableContract(
      "AtomicLockSettlerFactory",
      "initialize",
      [],
    )) as AtomicLockSettlerFactory;
  });

  async function deployAtomicLockSettler(
    entries: {
      contractAddress: string;
      lockId: string;
      spendInputs: Uint8Array | string;
      cancelInputs: Uint8Array | string;
      data: Uint8Array | string;
    }[],
    executor: Signer,
  ): Promise<AtomicLockSettler> {
    const createTx = await lockSettlerFactory.connect(executor).create(entries);
    const receipt = await createTx.wait();
    const deployedEvent = receipt?.logs
      .filter((log) => {
        try {
          const parsed = lockSettlerFactory.interface.parseLog(log);
          return parsed?.name === "SettlerDeployed";
        } catch {
          return false;
        }
      })
      .map((log) => lockSettlerFactory.interface.parseLog(log))[0];
    const atomicLockSettlerAddress = deployedEvent?.args[0] as string;
    return (await ethers.getContractAt(
      "AtomicLockSettler",
      atomicLockSettlerAddress,
    )) as AtomicLockSettler;
  }

  it("atomically spends 2 token locks from different users", async function () {
    const [minter, alice, bob, charlie, dave] = await ethers.getSigners();
    const aliceAddress = await alice.getAddress();
    const bobAddress = await bob.getAddress();
    const charlieAddress = await charlie.getAddress();
    const daveAddress = await dave.getAddress();

    // Deploy token and mint to alice and bob
    const TokenFactory = await ethers.getContractFactory("ERC20Lockable");
    const token = (await TokenFactory.connect(minter).deploy(
      "TestToken",
      "TEST",
    )) as ERC20Lockable;

    await token.connect(minter).mint(aliceAddress, ethers.parseEther("1000"));
    await token.connect(minter).mint(bobAddress, ethers.parseEther("1000"));

    // Alice creates a lock for 100 tokens to charlie
    const lockId1 = await createTokenLock(
      token,
      alice,
      ethers.parseEther("100"),
      charlieAddress,
    );

    // Bob creates a lock for 200 tokens to dave
    const lockId2 = await createTokenLock(
      token,
      bob,
      ethers.parseEther("200"),
      daveAddress,
    );

    const settler = await deployAtomicLockSettler(
      [
        {
          contractAddress: await token.getAddress(),
          lockId: lockId1,
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
        {
          contractAddress: await token.getAddress(),
          lockId: lockId2,
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
      ],
      alice,
    );

    // Delegate locks to the settler
    const settlerAddress = await settler.getAddress();
    await token
      .connect(alice)
      .delegateLock(lockId1, "0x", settlerAddress, "0x");
    await token.connect(bob).delegateLock(lockId2, "0x", settlerAddress, "0x");

    // Check initial balances
    expect(await token.balanceOf(charlieAddress)).to.equal(0);
    expect(await token.balanceOf(daveAddress)).to.equal(0);
    expect(await token.lockedBalanceOf(aliceAddress)).to.equal(
      ethers.parseEther("100"),
    );
    expect(await token.lockedBalanceOf(bobAddress)).to.equal(
      ethers.parseEther("200"),
    );

    await settler.connect(alice).spend();

    // Verify atomic execution
    expect(await settler.status()).to.equal(1); // Status.Spent

    // Verify tokens were transferred
    expect(await token.balanceOf(charlieAddress)).to.equal(
      ethers.parseEther("100"),
    );
    expect(await token.balanceOf(daveAddress)).to.equal(
      ethers.parseEther("200"),
    );

    // Verify locked balances cleared
    expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
    expect(await token.lockedBalanceOf(bobAddress)).to.equal(0);

    // Verify locks were deleted
    await expect(token.getLock(lockId1)).to.be.revertedWithCustomError(
      token,
      "LockNotActive",
    );
    await expect(token.getLock(lockId2)).to.be.revertedWithCustomError(
      token,
      "LockNotActive",
    );
  });

  it("reverts spend if any spendLock reverts (atomicity)", async function () {
    const [minter, alice, bob, charlie] = await ethers.getSigners();
    const aliceAddress = await alice.getAddress();
    const bobAddress = await bob.getAddress();
    const charlieAddress = await charlie.getAddress();

    // Deploy token and mint to alice and bob
    const TokenFactory = await ethers.getContractFactory("ERC20Lockable");
    const token = (await TokenFactory.connect(minter).deploy(
      "TestToken",
      "TEST",
    )) as ERC20Lockable;

    await token.connect(minter).mint(aliceAddress, ethers.parseEther("1000"));
    await token.connect(minter).mint(bobAddress, ethers.parseEther("1000"));

    // Alice creates a valid lock
    const lockId1 = await createTokenLock(
      token,
      alice,
      ethers.parseEther("100"),
      charlieAddress,
    );

    // Create a fake/invalid lock ID for Bob's entry to test atomicity
    const fakeLockId = ethers.keccak256(ethers.toUtf8Bytes("fake-lock"));

    const settler = await deployAtomicLockSettler(
      [
        {
          contractAddress: await token.getAddress(),
          lockId: lockId1, // Alice's valid lock
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
        {
          contractAddress: await token.getAddress(),
          lockId: fakeLockId, // Fake lock that doesn't exist
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
      ],
      alice,
    );

    // Only delegate Alice's valid lock
    const settlerAddress = await settler.getAddress();
    await token
      .connect(alice)
      .delegateLock(lockId1, "0x", settlerAddress, "0x");

    // Should revert because the fake lock doesn't exist
    await expect(settler.connect(alice).spend()).to.be.revertedWithCustomError(
      token,
      "LockNotActive",
    );

    // Verify nothing was executed
    expect(await settler.status()).to.equal(0); // Status.Pending

    // Verify no transfer occurred
    expect(await token.balanceOf(charlieAddress)).to.equal(0);

    // Verify Alice's lock still exists
    expect(await token.lockedBalanceOf(aliceAddress)).to.equal(
      ethers.parseEther("100"),
    );
  });

  it("cancels all locks", async function () {
    const [minter, alice, bob, charlie, dave] = await ethers.getSigners();
    const aliceAddress = await alice.getAddress();
    const bobAddress = await bob.getAddress();
    const charlieAddress = await charlie.getAddress();
    const daveAddress = await dave.getAddress();

    // Deploy token and mint to alice and bob
    const TokenFactory = await ethers.getContractFactory("ERC20Lockable");
    const token = (await TokenFactory.connect(minter).deploy(
      "TestToken",
      "TEST",
    )) as ERC20Lockable;

    await token.connect(minter).mint(aliceAddress, ethers.parseEther("1000"));
    await token.connect(minter).mint(bobAddress, ethers.parseEther("1000"));

    // Create locks
    const lockId1 = await createTokenLock(
      token,
      alice,
      ethers.parseEther("100"),
      charlieAddress,
    );

    const lockId2 = await createTokenLock(
      token,
      bob,
      ethers.parseEther("200"),
      daveAddress,
    );

    const settler = await deployAtomicLockSettler(
      [
        {
          contractAddress: await token.getAddress(),
          lockId: lockId1,
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
        {
          contractAddress: await token.getAddress(),
          lockId: lockId2,
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
      ],
      alice,
    );

    // Delegate locks to the settler
    const settlerAddress = await settler.getAddress();
    await token
      .connect(alice)
      .delegateLock(lockId1, "0x", settlerAddress, "0x");
    await token.connect(bob).delegateLock(lockId2, "0x", settlerAddress, "0x");

    // Verify locked balances before cancel
    expect(await token.lockedBalanceOf(aliceAddress)).to.equal(
      ethers.parseEther("100"),
    );
    expect(await token.lockedBalanceOf(bobAddress)).to.equal(
      ethers.parseEther("200"),
    );

    await settler.connect(alice).cancel();

    // Should succeed with best-effort cancellation
    expect(await settler.status()).to.equal(2); // Status.Cancelled

    // Verify locked balances cleared
    expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
    expect(await token.lockedBalanceOf(bobAddress)).to.equal(0);

    // Verify no transfers occurred (tokens just unlocked)
    expect(await token.balanceOf(charlieAddress)).to.equal(0);
    expect(await token.balanceOf(daveAddress)).to.equal(0);

    // Verify locks were deleted
    await expect(token.getLock(lockId1)).to.be.revertedWithCustomError(
      token,
      "LockNotActive",
    );
    await expect(token.getLock(lockId2)).to.be.revertedWithCustomError(
      token,
      "LockNotActive",
    );
  });

  it("best-effort cancel: partial failure then retry after late delegation", async function () {
    const [minter, alice, bob, charlie, dave] = await ethers.getSigners();
    const aliceAddress = await alice.getAddress();
    const bobAddress = await bob.getAddress();
    const charlieAddress = await charlie.getAddress();
    const daveAddress = await dave.getAddress();

    // Deploy token and mint to alice and bob
    const TokenFactory = await ethers.getContractFactory("ERC20Lockable");
    const token = (await TokenFactory.connect(minter).deploy(
      "TestToken",
      "TEST",
    )) as ERC20Lockable;

    await token.connect(minter).mint(aliceAddress, ethers.parseEther("1000"));
    await token.connect(minter).mint(bobAddress, ethers.parseEther("1000"));

    // Create locks
    const lockId1 = await createTokenLock(
      token,
      alice,
      ethers.parseEther("100"),
      charlieAddress,
    );
    const lockId2 = await createTokenLock(
      token,
      bob,
      ethers.parseEther("200"),
      daveAddress,
    );

    // Verify initial state
    expect(await token.lockedBalanceOf(aliceAddress)).to.equal(
      ethers.parseEther("100"),
    );
    expect(await token.lockedBalanceOf(bobAddress)).to.equal(
      ethers.parseEther("200"),
    );

    const settler = await deployAtomicLockSettler(
      [
        {
          contractAddress: await token.getAddress(),
          lockId: lockId1,
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
        {
          contractAddress: await token.getAddress(),
          lockId: lockId2,
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
      ],
      alice,
    );

    const settlerAddress = await settler.getAddress();

    // SCENARIO: Alice delegates her lock, but Bob forgets to delegate his
    await token
      .connect(alice)
      .delegateLock(lockId1, "0x", settlerAddress, "0x");

    // FIRST CANCEL: Alice's succeeds, Bob's fails silently
    await settler.connect(alice).cancel();

    // Verify best-effort result: Alice's lock cancelled, Bob's still exists
    expect(await settler.status()).to.equal(2); // Status.Cancelled
    expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0); // Alice's unlocked
    expect(await token.lockedBalanceOf(bobAddress)).to.equal(
      ethers.parseEther("200"), // Bob's still locked
    );

    // Verify Alice's lock was deleted but Bob's still exists
    await expect(token.getLock(lockId1)).to.be.revertedWithCustomError(
      token,
      "LockNotActive",
    );
    await token.getLock(lockId2); // Bob's lock still active

    // LATER: Bob accidentally delegates his lock to the original settler
    await token.connect(bob).delegateLock(lockId2, "0x", settlerAddress, "0x");

    // SECOND CANCEL: Now Bob can cancel his lock
    await settler.connect(bob).cancel();

    // Verify final state: Bob's lock is now cancelled too
    expect(await settler.status()).to.equal(2); // Status.Cancelled
    expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
    expect(await token.lockedBalanceOf(bobAddress)).to.equal(0); // Now Bob's is unlocked too

    // Verify Bob's lock is now deleted
    await expect(token.getLock(lockId2)).to.be.revertedWithCustomError(
      token,
      "LockNotActive",
    );

    // Verify no transfers occurred (tokens just unlocked)
    expect(await token.balanceOf(charlieAddress)).to.equal(0);
    expect(await token.balanceOf(daveAddress)).to.equal(0);
  });

  it("should not allow spend after cancel", async function () {
    const [owner1] = await ethers.getSigners();

    const TokenFactory = await ethers.getContractFactory("ERC20Lockable");
    const token = await TokenFactory.deploy("TestToken", "TEST");

    const settler = await deployAtomicLockSettler(
      [
        {
          contractAddress: await token.getAddress(),
          lockId: ethers.keccak256(ethers.toUtf8Bytes("dummy-lock")),
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
      ],
      owner1,
    );

    await settler.connect(owner1).cancel();
    expect(await settler.status()).to.equal(2); // Status.Cancelled

    await expect(
      settler.connect(owner1).spend(),
    ).to.be.revertedWithCustomError(settler, "SettlerNotPending");
  });

  it("should not allow cancel after spend", async function () {
    const [minter, owner1, , recipient] = await ethers.getSigners();
    const owner1Address = await owner1.getAddress();
    const recipientAddress = await recipient.getAddress();

    const TokenFactory = await ethers.getContractFactory("ERC20Lockable");
    const token = (await TokenFactory.connect(minter).deploy(
      "TestToken",
      "TEST",
    )) as ERC20Lockable;
    await token.connect(minter).mint(owner1Address, ethers.parseEther("100"));

    const lockId = await createTokenLock(
      token,
      owner1,
      ethers.parseEther("50"),
      recipientAddress,
    );

    const settler = await deployAtomicLockSettler(
      [
        {
          contractAddress: await token.getAddress(),
          lockId: lockId,
          spendInputs: "0x",
          cancelInputs: "0x",
          data: "0x",
        },
      ],
      owner1,
    );

    const settlerAddress = await settler.getAddress();
    await token
      .connect(owner1)
      .delegateLock(lockId, "0x", settlerAddress, "0x");

    await settler.connect(owner1).spend();
    expect(await settler.status()).to.equal(1); // Status.Spent

    await expect(
      settler.connect(owner1).cancel(),
    ).to.be.revertedWithCustomError(settler, "SettlerNotPending");
  });
});
