import { expect } from "chai";
import { randomBytes } from "crypto";
import { Signer, TypedDataEncoder } from "ethers";
import hre from "hardhat";
import { Noto, NotoFactory } from "../../../typechain-types";

export async function newTransferHash(
  noto: Noto,
  inputs: string[],
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
    Transfer: [
      { name: "inputs", type: "bytes32[]" },
      { name: "outputs", type: "bytes32[]" },
      { name: "data", type: "bytes" },
    ],
  };
  const value = { inputs, outputs, data };
  return TypedDataEncoder.hash(domain, types, value);
}

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

export async function deployNotoInstance(
  notoFactory: NotoFactory,
  notary: string
) {
  const deployTx = await notoFactory.deploy(randomBytes32(), notary, "0x");
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
    expect(event?.name).to.equal("NotoTransfer");
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
    expect(event?.name).to.equal("NotoTransfer");
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
  inputs: string[],
  outputs: string[],
  lockedOutputs: string[],
  data: string
) {
  const tx = await noto
    .connect(notary)
    .lock(txId, inputs, outputs, lockedOutputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoLock");
    expect(event?.args.inputs).to.deep.equal(inputs);
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.lockedOutputs).to.deep.equal(lockedOutputs);
    expect(event?.args.data).to.deep.equal(data);
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
  lockedInputs: string[],
  lockedOutputs: string[],
  outputs: string[],
  data: string
) {
  const tx = await noto
    .connect(sender)
    .unlock(txId, lockedInputs, lockedOutputs, outputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoUnlock");
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
  notary: Signer,
  noto: Noto,
  lockedInputs: string[],
  unlockHash: string,
  data: string
) {
  const tx = await noto
    .connect(notary)
    .prepareUnlock(lockedInputs, unlockHash, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoUnlockPrepared");
    expect(event?.args.lockedInputs).to.deep.equal(lockedInputs);
    expect(event?.args.unlockHash).to.deep.equal(unlockHash);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const input of lockedInputs) {
    expect(await noto.isLocked(input)).to.equal(true);
  }
}

export async function doDelegateLock(
  txId: string,
  notary: Signer,
  noto: Noto,
  unlockHash: string,
  delegate: string,
  data: string
) {
  const tx = await noto
    .connect(notary)
    .delegateLock(txId, unlockHash, delegate, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoLockDelegated");
    expect(event?.args.delegate).to.deep.equal(delegate);
    expect(event?.args.data).to.deep.equal(data);
  }
}
