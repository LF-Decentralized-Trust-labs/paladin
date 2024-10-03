import { loadFixture } from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { expect } from "chai";
import { AbiCoder, ContractTransactionReceipt, Signer } from "ethers";
import hre, { ethers } from "hardhat";
import { NotoFactory, NotoSelfSubmit } from "../../../typechain-types";
import { fakeTXO, randomBytes32 } from "./Noto";

export async function prepareSignature(
  noto: NotoSelfSubmit,
  notary: Signer,
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
  return notary.signTypedData(domain, types, value);
}

export async function deployNotoInstance(
  notoFactory: NotoFactory,
  notary: string
) {
  const abi = AbiCoder.defaultAbiCoder();
  const Noto = await ethers.getContractFactory("NotoSelfSubmit");
  const notoImpl = await Noto.deploy();
  await notoFactory.registerImplementation("selfsubmit", notoImpl);
  const deployTx = await notoFactory.deployImplementation(
    "selfsubmit",
    randomBytes32(),
    "",
    "0x0000000000000000000000000000000000000000000000000000000000000000",
    notary
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

describe("NotoSelfSubmit", function () {
  async function deployNotoFixture() {
    const [notary, other] = await ethers.getSigners();
    const abi = AbiCoder.defaultAbiCoder();

    const NotoFactory = await ethers.getContractFactory("NotoFactory");
    const notoFactory = await NotoFactory.deploy();
    const Noto = await ethers.getContractFactory("Noto");
    const noto = Noto.attach(
      await deployNotoInstance(notoFactory, notary.address)
    );

    return { noto: noto as NotoSelfSubmit, notary, other };
  }

  async function doTransfer(
    notary: Signer,
    submitter: Signer,
    noto: NotoSelfSubmit,
    inputs: string[],
    outputs: string[],
    data: string
  ) {
    const signature = await prepareSignature(
      noto,
      notary,
      inputs,
      outputs,
      data
    );
    const tx = await noto
      .connect(submitter)
      .transfer(inputs, outputs, signature, data);
    const results: ContractTransactionReceipt | null = await tx.wait();

    for (const log of results?.logs || []) {
      const event = noto.interface.parseLog(log as any);
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

  it("UTXO lifecycle and double-spend protections", async function () {
    const { noto, notary, other } = await loadFixture(deployNotoFixture);

    const txo1 = fakeTXO();
    const txo2 = fakeTXO();
    const txo3 = fakeTXO();

    // Make two UTXOs
    await doTransfer(notary, other, noto, [], [txo1, txo2], randomBytes32());

    // Check for double-mint protection
    await expect(
      doTransfer(notary, other, noto, [], [txo1], randomBytes32())
    ).rejectedWith("NotoInvalidOutput");

    // Check for spend unknown protection
    await expect(
      doTransfer(notary, other, noto, [txo3], [], randomBytes32())
    ).rejectedWith("NotoInvalidInput");

    // Spend one
    await doTransfer(notary, other, noto, [txo1], [txo3], randomBytes32());

    // Check for double-spend protection
    await expect(
      doTransfer(notary, other, noto, [txo1], [txo3], randomBytes32())
    ).rejectedWith("NotoInvalidInput");

    // Spend another
    await doTransfer(notary, other, noto, [txo2], [], randomBytes32());

    // Spend the last one
    await doTransfer(notary, other, noto, [txo3], [], randomBytes32());
  });
});
