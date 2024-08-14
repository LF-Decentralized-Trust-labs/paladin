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
      const abiCoder = hre.ethers.AbiCoder.defaultAbiCoder();
      const { simDomain, notary } = await loadFixture(simDomainSetup);
      
      // Invoke the factory function to create the actual SIMToken
      const SIMTokenFactory = await hre.ethers.getContractFactory("SIMToken");
      const deployTxId = randBytes32();
      const factoryTX = await (await simDomain.newSIMTokenNotarized(deployTxId, notary, 'my/notary')).wait();
      expect(factoryTX?.logs).to.have.lengthOf(1);

      // It should emit an event declaring its existence, linking back to the domain
      const deployEvent = SIMTokenFactory.interface.parseLog(factoryTX!.logs[0])
      expect(deployEvent?.name).to.equal('PaladinNewSmartContract_V0');
      expect(deployEvent?.args.toObject()["txId"]).to.equal(deployTxId);
      expect(deployEvent?.args.toObject()["domain"]).to.equal(await simDomain.getAddress());
      expect(deployEvent?.args.toObject()["data"]).to.equal(abiCoder.encode(['string'], ['my/notary']));
      const deployedAddr = factoryTX!.logs[0].address;
      
      // Now we have the token - create a client for it using the notary address
      const SIMToken = await hre.ethers.getContractAt("SIMToken", deployedAddr);
      const simToken = SIMToken.connect(notary);

      // Invoke against it an expect an event
      const SINGLE_FUNCTION_SELECTOR = hre.ethers.keccak256(hre.ethers.toUtf8Bytes("SIMToken()"));
      const txID = randBytes32();
      const signature = randBytes32();
      const inputs = [randBytes32(), randBytes32()];
      const outputs = [randBytes32(), randBytes32()];
      const payload = abiCoder.encode(['bytes32', 'bytes32[]', 'bytes32[]'], [signature, inputs, outputs]);
      await expect(simToken.paladinExecute_V0(txID, SINGLE_FUNCTION_SELECTOR, payload)).to.
        emit(simToken, "PaladinPrivateTransaction_V0")
        .withArgs(
          txID,
          inputs,
          outputs,
          signature
        );
    });
  });
});
