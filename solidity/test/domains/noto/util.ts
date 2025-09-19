import { expect } from "chai";
import { randomBytes } from "crypto";
import { Signer, TypedDataEncoder, ZeroAddress } from "ethers";
import hre, { ethers } from "hardhat";
import { Noto, NotoFactory } from "../../../typechain-types";

export async function newUnlockHash(
  noto: Noto,
  lockedInputs: string[],
  lockedOutputs: string[],
  outputs: string[],
  data: string
) {
  const domain = {
    name: "noto",
    version: "0.0.1",
    chainId: hre.network.config.chainId,
    verifyingContract: await noto.getAddress(),
  };
  const types = {
    Unlock: [
      { name: "lockedInputs", type: "bytes32[]" },
      { name: "lockedOutputs", type: "bytes32[]" },
      { name: "outputs", type: "bytes32[]" },
      { name: "data", type: "bytes" },
    ],
  };
  const value = { lockedInputs, lockedOutputs, outputs, data };
  return TypedDataEncoder.hash(domain, types, value);
}

export function randomBytes32() {
  return "0x" + Buffer.from(randomBytes(32)).toString("hex");
}

export function fakeTXO() {
  return randomBytes32();
}

export function createLockOptions(unlockHash?: string, expiration?: number) {
  const options = {
    unlockHash: unlockHash || "0x0000000000000000000000000000000000000000000000000000000000000000",
    expiration: expiration || 0
  };
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32,uint256)"],
    [[options.unlockHash, options.expiration]]
  );
}

export async function deployNotoInstance(
  notoFactory: NotoFactory,
  notary: string
) {
  const deployTx = await notoFactory.deploy(randomBytes32(), "NOTO", "NOTO", notary, "0x");
  const deployReceipt = await deployTx.wait();
  const deployEvent = deployReceipt?.logs.find(
    (l) =>
      notoFactory.interface.parseLog(l)?.name ===
      "PaladinRegisterSmartContract_V0"
  );
  expect(deployEvent).to.exist;
  return deployEvent && "args" in deployEvent ? deployEvent.args.instance : "";
}

export async function doTransfer(
  txId: string,
  notary: Signer,
  noto: Noto,
  inputs: string[],
  outputs: string[],
  data: string
) {
  const tx = await noto
    .connect(notary)
    .transfer(txId, inputs, outputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("Transfer");
    expect(event?.args.inputs).to.deep.equal(inputs);
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const input of inputs) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
}

export async function doMint(
  txId: string,
  notary: Signer,
  noto: Noto,
  outputs: string[],
  data: string
) {
  const tx = await noto.connect(notary).transfer(txId, [], outputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("Transfer");
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
}

export async function doLock(
  txId: string,
  notary: Signer,
  noto: Noto,
  lockId: string,
  inputs: string[],
  outputs: string[],
  lockedOutputs: string[],
  data: string,
  options?: string
) {
  const tx = await noto
    .connect(notary)
    .lock(txId,
      lockId,
      { inputs, outputs, lockedOutputs },
      ZeroAddress,
      options || "0x",
      "0x",
      data
    );
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("Lock");
    expect(event?.args.states.inputs).to.deep.equal(inputs);
    expect(event?.args.states.outputs).to.deep.equal(outputs);
    expect(event?.args.states.lockedOutputs).to.deep.equal(lockedOutputs);
    expect(event?.args.data).to.equal(data);
  }
  for (const input of inputs) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
  for (const output of lockedOutputs) {
    expect(await noto.isLocked(output)).to.equal(true);
    expect(await noto.isUnspent(output)).to.equal(false);
  }
}

export async function doUnlock(
  txId: string,
  sender: Signer,
  noto: Noto,
  lockId: string,
  lockedInputs: string[],
  lockedOutputs: string[],
  outputs: string[],
  data: string
) {
  const tx = await noto
    .connect(sender)
    .transferLocked(txId, lockId, lockedInputs, lockedOutputs, outputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("TransferLocked");
    expect(event?.args.lockedInputs).to.deep.equal(lockedInputs);
    expect(event?.args.lockedOutputs).to.deep.equal(lockedOutputs);
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const input of lockedInputs) {
    expect(await noto.isLocked(input)).to.equal(false);
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of lockedOutputs) {
    expect(await noto.isLocked(output)).to.equal(true);
    expect(await noto.isUnspent(output)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
}

export async function doPrepareUnlock(
  txId: string,
  notary: Signer,
  noto: Noto,
  lockId: string,
  lockedInputs: string[],
  unlockHash: string,
  data: string
) {
  const options = createLockOptions(unlockHash, 0);
  const tx = await noto
    .connect(notary)
    .setLockOptions(txId, lockId, lockedInputs, options, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("LockUpdate");
    expect(event?.args.lockedInputs).to.deep.equal(lockedInputs);
    expect(event?.args.options).to.equal(options);
    expect(event?.args.data).to.equal(data);
  }
  for (const input of lockedInputs) {
    expect(await noto.isLocked(input)).to.equal(true);
  }
}

export async function doDelegateLock(
  txId: string,
  notary: Signer,
  noto: Noto,
  lockId: string,
  delegate: string,
  data: string
) {
  const tx = await noto
    .connect(notary)
    .delegateLock(txId, lockId, delegate, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("LockDelegation");
    expect(event?.args.delegate).to.deep.equal(delegate);
    expect(event?.args.data).to.deep.equal(data);
  }
}
