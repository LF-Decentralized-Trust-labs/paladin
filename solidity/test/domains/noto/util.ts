import { expect } from "chai";
import { randomBytes } from "crypto";
import { Signer, TypedDataEncoder, ZeroHash } from "ethers";
import hre, { ethers } from "hardhat";
import { Noto, NotoFactory } from "../../../typechain-types";

export interface LockParams {
  txId: string;
  inputs: string[];
  outputs: string[];
  lockedOutputs: string[];
  proof: string;
  options: string;
}

export async function newUnlockHash(
  noto: Noto,
  txId: string,
  lockedInputs: string[],
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
      { name: "txId", type: "bytes32" },
      { name: "lockedInputs", type: "bytes32[]" },
      { name: "outputs", type: "bytes32[]" },
      { name: "data", type: "bytes" },
    ],
  };
  const value = { txId, lockedInputs, outputs, data };
  return TypedDataEncoder.hash(domain, types, value);
}

export function randomBytes32() {
  return "0x" + Buffer.from(randomBytes(32)).toString("hex");
}

export function fakeTXO() {
  return randomBytes32();
}

export function createLockOptions(spendTxId: string, spendHash: string, cancelHash: string) {
  const options = {
    spendTxId: spendTxId,
    spendHash: spendHash,
    cancelHash: cancelHash,
  };
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32,bytes32,bytes32)"],
    [[options.spendTxId, options.spendHash, options.cancelHash]]
  );
}

export async function deployNotoInstance(
  notoFactory: NotoFactory,
  notary: string
) {
  const deployTx = await notoFactory.deploy(
    randomBytes32(),
    "NOTO",
    "NOTO",
    notary,
    "0x"
  );
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
  notary: Signer,
  noto: Noto,
  params: LockParams,
  data: string
) {
  const tx = await noto.connect(notary).createLock(params, data);
  const results = await tx.wait();
  expect(results).to.exist;
  const lockId = await noto.computeLockId(params);

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("LockCreated");
    expect(event?.args.inputs).to.deep.equal(params.inputs);
    expect(event?.args.outputs).to.deep.equal(params.outputs);
    expect(event?.args.lockedOutputs).to.deep.equal(params.lockedOutputs);
    expect(event?.args.data).to.equal(data);
  }
  for (const input of params.inputs) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of params.outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
  for (const output of params.lockedOutputs) {
    expect(await noto.getLockId(output)).to.equal(lockId);
    expect(await noto.isUnspent(output)).to.equal(false);
  }
  return lockId;
}

export async function doUnlock(
  txId: string,
  sender: Signer,
  noto: Noto,
  lockId: string,
  lockedInputs: string[],
  outputs: string[],
  data: string
) {
  const unlockParams = {
    txId: txId,
    inputs: lockedInputs,
    outputs: outputs,
    data: data,
  };
  const encodedParams = ethers.AbiCoder.defaultAbiCoder().encode(
    ["bytes32", "bytes32[]", "bytes32[]", "bytes"],
    [
      unlockParams.txId,
      unlockParams.inputs,
      unlockParams.outputs,
      unlockParams.data,
    ]
  );
  const tx = await noto.connect(sender).spendLock(lockId, encodedParams);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("LockSpent");
    expect(event?.args.lockId).to.equal(lockId);
  }
  for (const input of lockedInputs) {
    expect(await noto.getLockId(input)).to.equal(
      "0x0000000000000000000000000000000000000000000000000000000000000000"
    );
    expect(await noto.isUnspent(input)).to.equal(false);
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
  spendTxId: string,
  spendHash: string,
  cancelHash: string,
  data: string,
) {
  const options = createLockOptions(spendTxId , spendHash, cancelHash);
  const params = {
    txId: txId,
    lockedInputs: lockedInputs,
    proof: "0x",
    options: options,
  };
  const tx = await noto.connect(notary).updateLock(lockId, params, data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("LockUpdated");
    expect(event?.args.lockedInputs).to.deep.equal(lockedInputs);
    expect(event?.args.proof).to.equal("0x");
    expect(event?.args.data).to.equal(data);
  }
  for (const input of lockedInputs) {
    expect(await noto.getLockId(input)).to.equal(lockId);
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
  const delegateLockParams = {
    txId: txId,
    data: data,
  };
  const encodedParams = ethers.AbiCoder.defaultAbiCoder().encode(
    ["bytes32", "bytes"],
    [delegateLockParams.txId, delegateLockParams.data]
  );
  const tx = await noto.connect(notary).delegateLock(lockId, delegate, encodedParams);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("LockDelegated");
    expect(event?.args.to).to.deep.equal(delegate);
    expect(event?.args.data).to.deep.equal(encodedParams);
  }
  const lockInfo = await noto.getLock(lockId);
  expect(lockInfo.spender).to.equal(delegate);
}
