import {
  loadFixture
} from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { expect } from "chai";
import hre from "hardhat";
import { PentePrivacyGroup } from "../../typechain-types";
import { AbiCoder, concat, TypedDataEncoder } from "ethers";
const abiCoder = AbiCoder.defaultAbiCoder();

enum PenteConfigID {
  Endorsement_V0 = 0x00010000,
}

export async function newTransitionHash(
  privacyGroup: PentePrivacyGroup,
  accounts: string[],
  oldStates: string[],
  newStates: string[]
) {
  const domain = {
    name: "pente",
    version: "0.0.1",
    chainId: hre.network.config.chainId,
    verifyingContract: await privacyGroup.getAddress(),
  };
  const types = {
    Transfer: [
      { name: "accounts", type: "bytes32[]" },
      { name: "oldStates", type: "bytes32[]" },
      { name: "newStates", type: "bytes32[]" },
    ],
  };
  const value = { accounts, oldStates, newStates };
  return {
    hash: TypedDataEncoder.hash(domain, types, value),
  };
}

describe("PentePrivacyGroup", function () {

  async function pentePrivacyGroupSetup() {
    const [deployer, endorser1, endorser2, endorser3] = await hre.ethers.getSigners();

    const configType: PenteConfigID = PenteConfigID.Endorsement_V0;
    const configTypeBytes = '0x' + (configType).toString(16).padStart(8, '0');
    const config = abiCoder.encode(
      ["uint", "address[]"],
      [3, [endorser1.address, endorser2.address, endorser3.address]],
    );
    const configBytes = concat([configTypeBytes, config]);

    const PenteFactory = await hre.ethers.getContractFactory("PenteFactory");
    const penteFactory = await (await PenteFactory.connect(deployer).deploy()).waitForDeployment();

    // Invoke the factory function to create the actual PentePrivacyGroup
    const deployTxId = randBytes32();
    const factoryTX = await (await penteFactory.connect(deployer).newPrivacyGroup(deployTxId, configBytes)).wait();
    expect(factoryTX?.logs).to.have.lengthOf(2);
    
    // It should emit an event declaring its existence, linking back to the domain
    const PentePrivacyGroup = await hre.ethers.getContractFactory("PentePrivacyGroup");
    const deployEvent = PentePrivacyGroup.interface.parseLog(factoryTX!.logs[0])
    expect(deployEvent?.name).to.equal('PaladinNewSmartContract_V0');
    expect(deployEvent?.args.toObject()["txId"]).to.equal(deployTxId);
    expect(deployEvent?.args.toObject()["domain"]).to.equal(await penteFactory.getAddress());
    expect(deployEvent?.args.toObject()["data"]).to.equal(configBytes);
    const deployedAddr = factoryTX!.logs[0].address;

    return { deployedAddr, endorser1, endorser2, endorser3 };
  }

  const randBytes32 = () => "0x" + Buffer.from(hre.ethers.randomBytes(32)).toString('hex');

  describe("Factory", function () {
    it("should deploy a new smart contract and emit an event", async function () {

      await pentePrivacyGroupSetup();

      // // Invoke against it an expect an event
      // const SINGLE_FUNCTION_SELECTOR = hre.ethers.keccak256(hre.ethers.toUtf8Bytes("SIMToken()"));
      // const txID = randBytes32();
      // const signature = randBytes32();
      // const inputs = [randBytes32(), randBytes32()];
      // const outputs = [randBytes32(), randBytes32()];
      // const payload = abiCoder.encode(['bytes32', 'bytes32[]', 'bytes32[]'], [signature, inputs, outputs]);
      // await expect(simToken.paladinExecute_V0(txID, SINGLE_FUNCTION_SELECTOR, payload)).to.
      //   emit(simToken, "PaladinPrivateTransaction_V0")
      //   .withArgs(
      //     txID,
      //     inputs,
      //     outputs,
      //     signature
      //   );
    });
  });
});
