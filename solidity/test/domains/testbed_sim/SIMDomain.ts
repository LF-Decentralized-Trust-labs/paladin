import {
  loadFixture
} from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { expect } from "chai";
import hre from "hardhat";

describe("SIMDomain", function () {

  async function simDomainSetup() {
    const [deployer, notary] = await hre.ethers.getSigners();

    const SIMDomain = await hre.ethers.getContractFactory("SIMDomain");
    const simDomain = await SIMDomain.connect(deployer).deploy();

    return { simDomain, notary };
  }

  const randBytes32 = () => "0x" + Buffer.from(hre.ethers.randomBytes(32)).toString('hex');

  describe("Factory", function () {
    it("should deploy a new smart contract and emit an event", async function () {
      const { simDomain, notary } = await loadFixture(simDomainSetup);
      const abiCoder = hre.ethers.AbiCoder.defaultAbiCoder();
      const SIMDomain = await hre.ethers.getContractFactory("SIMDomain");
      
      // Invoke the factory function to create the actual SIMToken
      const SIMTokenFactory = await hre.ethers.getContractFactory("SIMToken");
      const deployTxId = randBytes32();
      const factoryTX = await (await simDomain.newSIMTokenNotarized(deployTxId, notary, 'my/notary')).wait();
      expect(factoryTX?.logs).to.have.lengthOf(1);

      // It should emit an event declaring its existence, linking back to the domain
      const deployEvent = SIMDomain.interface.parseLog(factoryTX!.logs[0])
      expect(deployEvent?.name).to.equal('PaladinRegisterSmartContract_V0');
      expect(deployEvent?.args.toObject()["txId"]).to.equal(deployTxId);
      expect(deployEvent?.args.toObject()["data"]).to.equal(abiCoder.encode(['string'], ['my/notary']));
      const deployedAddr = deployEvent?.args.toObject()["instance"];
      
      // Now we have the token - create a client for it using the notary address
      const SIMToken = await hre.ethers.getContractAt("SIMToken", deployedAddr);
      const simToken = SIMToken.connect(notary);

      // Invoke against it an expect an event
      const SINGLE_FUNCTION_SELECTOR = hre.ethers.keccak256(hre.ethers.toUtf8Bytes("SIMToken()"));
      const txId = randBytes32();
      const signature = randBytes32();
      const inputs = [randBytes32(), randBytes32()];
      const outputs = [randBytes32(), randBytes32()];
      const payload = abiCoder.encode(['bytes32', 'bytes32[]', 'bytes32[]'], [signature, inputs, outputs]);
      await expect(simToken.paladinExecute_V0(txId, SINGLE_FUNCTION_SELECTOR, payload)).to.
        emit(simToken, "UTXOTransfer")
        .withArgs(
          txId,
          inputs,
          outputs,
          signature
        );
    });
  });
});
