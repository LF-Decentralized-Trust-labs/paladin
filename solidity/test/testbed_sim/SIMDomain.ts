import {
  time,
  loadFixture,
} from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { anyValue } from "@nomicfoundation/hardhat-chai-matchers/withArgs";
import { expect } from "chai";
import hre from "hardhat";
import { SIMToken__factory } from "../../typechain-types";

describe("SIMDomain", function () {

  async function simDomainSetup() {
    const [deployer, notary] = await hre.ethers.getSigners();

    const SIMDomain = await hre.ethers.getContractFactory("SIMDomain");
    const simDomain = await SIMDomain.connect(deployer).deploy();

    return { simDomain, notary };
  }

  describe("Factory", function () {
    it("should deploy a new smart contract and emit an event", async function () {
      const { simDomain, notary } = await loadFixture(simDomainSetup);

      let capturedAddr = "";
      const captureAddress = (addr: string): boolean => {
        capturedAddr = addr;
        return hre.ethers.isAddress(addr)
      }

      await expect(simDomain.newSIMTokenNotarized(notary)).to.
        emit(simDomain, "PaladinNewSmartContract_V0")
        .withArgs(captureAddress);
      
      const SIMToken = await hre.ethers.getContractAt("SIMToken", capturedAddr);
      const simToken = SIMToken.connect(notary);

      const SINGLE_FUNCTION_SELECTOR = hre.ethers.keccak256(hre.ethers.toUtf8Bytes("SIMToken()"));
      const randBytes32 = () => "0x" + Buffer.from(hre.ethers.randomBytes(32)).toString('hex');
      const txID = randBytes32();
      const signature = randBytes32();
      const inputs = [randBytes32(), randBytes32()];
      const outputs = [randBytes32(), randBytes32()];
      const abiCoder = hre.ethers.AbiCoder.defaultAbiCoder();
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
