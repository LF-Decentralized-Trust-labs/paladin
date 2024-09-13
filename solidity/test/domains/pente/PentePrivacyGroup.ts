import { expect } from "chai";
import { AbiCoder, concat, Signer, TypedDataEncoder } from "ethers";
import hre from "hardhat";
import { PentePrivacyGroup } from "../../typechain-types";
const abiCoder = AbiCoder.defaultAbiCoder();

enum PenteConfigID {
  Endorsement_V0 = 0x00010000,
}

export async function newTransitionEIP712(
  privacyGroup: PentePrivacyGroup,
  inputs: string[],
  reads: string[],
  outputs: string[],
  signers: Signer[]
) {
  const domain = {
    name: "pente",
    version: "0.0.1",
    chainId: hre.network.config.chainId,
    verifyingContract: await privacyGroup.getAddress(),
  };
  const types = {
    Transition: [
      { name: "inputs",  type: "bytes32[]" },
      { name: "reads",   type: "bytes32[]" },
      { name: "outputs", type: "bytes32[]" },
    ],
  };
  const value = { inputs, reads, outputs };
  const hash = TypedDataEncoder.hash(domain, types, value);
  const signatures: string[] = [];
  for (const signer of signers) {
    signatures.push(await signer.signTypedData(domain, types, value));
  }
  return { hash, signatures }
}

describe("PentePrivacyGroup", function () {

  async function pentePrivacyGroupSetup() {
    const [deployer, endorser1, endorser2, endorser3] = await hre.ethers.getSigners();

    const configType: PenteConfigID = PenteConfigID.Endorsement_V0;
    const configTypeBytes = '0x' + (configType).toString(16).padStart(8, '0');
    const config = abiCoder.encode(
      ["string", "uint", "address[]"],
      ["shanghai", 3, [endorser1.address, endorser2.address, endorser3.address]],
    );
    const configBytes = concat([configTypeBytes, config]);

    const PenteFactory = await hre.ethers.getContractFactory("PenteFactory");
    const penteFactory = await (await PenteFactory.connect(deployer).deploy()).waitForDeployment();

    // Invoke the factory function to create the actual PentePrivacyGroup
    const deployTxId = randBytes32();
    const factoryTX = await (await penteFactory.connect(deployer).newPrivacyGroup(deployTxId, configBytes)).wait();
    expect(factoryTX?.logs).to.have.lengthOf(2);
    
    // It should emit an event declaring its existence, linking back to the domain
    const deployEvent = PenteFactory.interface.parseLog(factoryTX!.logs[1])
    expect(factoryTX!.logs[1].address).to.equal(await penteFactory.getAddress());
    expect(deployEvent?.name).to.equal('PaladinRegisterSmartContract_V0');
    expect(deployEvent?.args.toObject()["txId"]).to.equal(deployTxId);
    expect(deployEvent?.args.toObject()["data"]).to.equal(configBytes);
    const privacyGroupAddress = deployEvent?.args.toObject()["instance"];
    const privacyGroup = await hre.ethers.getContractAt("PentePrivacyGroup", privacyGroupAddress);

    return { privacyGroup, endorsers: [endorser1, endorser2, endorser3], deployer };
  }

  const randBytes32 = () => "0x" + Buffer.from(hre.ethers.randomBytes(32)).toString('hex');
  
  it("successful transitions with full endorsement", async function () {

    const { privacyGroup, endorsers } = await pentePrivacyGroupSetup();

    const stateSet1    = [randBytes32(),randBytes32(),randBytes32()];

    // Build an en
    const { signatures: endorsements1 } =
      await newTransitionEIP712(privacyGroup, [], [], stateSet1, endorsers)
    const tx1ID = randBytes32();

    await expect(privacyGroup.transition(tx1ID, [], [], stateSet1, endorsements1)).to.
      emit(privacyGroup, "UTXOTransfer")
      .withArgs(
        tx1ID,
        [],
        stateSet1,
        "0x"
      );

    const stateSet2           = [randBytes32(),randBytes32(),randBytes32()];
    const inputs2             = [stateSet1[1]]
    const reads2              = [stateSet1[0], stateSet1[2]]
    const { signatures: endorsements2 } =
      await newTransitionEIP712(privacyGroup, inputs2, reads2, stateSet2,
        /* mix up the endorser order */
        [ endorsers[1], endorsers[0], endorsers[2] ]
      )
    const tx2ID = randBytes32();

    await expect(privacyGroup.transition(tx2ID, inputs2, reads2, stateSet2, endorsements2)).to.
      emit(privacyGroup, "UTXOTransfer")
      .withArgs(
        tx2ID,
        inputs2,
        stateSet2,
        "0x"
      );      

  });

  it("incomplete endorsements", async function () {

    const { privacyGroup, endorsers } = await pentePrivacyGroupSetup();

    const stateSet1           = [randBytes32(),randBytes32(),randBytes32()];

    // Build an en
    const { signatures: endorsements1 } =
      await newTransitionEIP712(privacyGroup, [], [], stateSet1, [endorsers[0],endorsers[1]])
    const tx1ID = randBytes32();

    await expect(privacyGroup.transition(tx1ID, [], [], stateSet1, endorsements1)).to.
      revertedWithCustomError(privacyGroup, "PenteEndorsementThreshold").withArgs(2, 3);

  });

  it("invalid endorsement", async function () {

    const { privacyGroup, deployer } = await pentePrivacyGroupSetup();

    const stateSet1           = [randBytes32(),randBytes32(),randBytes32()];

    const { signatures: endorsements1 } =
      await newTransitionEIP712(privacyGroup, [], [], stateSet1, [deployer])
    const tx1ID = randBytes32();

    await expect(privacyGroup.transition(tx1ID, [], [], stateSet1, endorsements1)).to.
      revertedWithCustomError(privacyGroup, "PenteInvalidEndorser").withArgs(deployer.address);

  });

  it("invalid input state", async function () {

    const { privacyGroup, endorsers } = await pentePrivacyGroupSetup();

    const stateSet1           = [randBytes32(),randBytes32(),randBytes32()];
    const stateSet2           = [randBytes32(),randBytes32(),randBytes32()];

    const { signatures: endorsements1 } =
      await newTransitionEIP712(privacyGroup, stateSet1, [], stateSet2, endorsers)
    const tx1ID = randBytes32();

    await expect(privacyGroup.transition(tx1ID, stateSet1, [], stateSet2, endorsements1)).to.
      revertedWithCustomError(privacyGroup, "PenteInputNotAvailable").withArgs(stateSet1[0]);

  });

  it("invalid read state", async function () {

    const { privacyGroup, endorsers } = await pentePrivacyGroupSetup();

    const stateSet1           = [randBytes32(),randBytes32(),randBytes32()];
    const stateSet2           = [randBytes32(),randBytes32(),randBytes32()];

    const { signatures: endorsements1 } =
      await newTransitionEIP712(privacyGroup, [], stateSet1, stateSet2, endorsers)
    const tx1ID = randBytes32();

    await expect(privacyGroup.transition(tx1ID, [], stateSet1, stateSet2, endorsements1)).to.
      revertedWithCustomError(privacyGroup, "PenteReadNotAvailable").withArgs(stateSet1[0]);

  });

  it("invalid double output", async function () {

    const { privacyGroup, endorsers } = await pentePrivacyGroupSetup();

    const stateSet1           = [randBytes32(),randBytes32(),randBytes32()];
    const stateSet2           = [randBytes32(),randBytes32(),randBytes32(), stateSet1[0]];
    const inputs2             = [stateSet1[1], stateSet1[2]];

    const { signatures: endorsements1 } =
      await newTransitionEIP712(privacyGroup, [], [], stateSet1, endorsers)
    const tx1ID = randBytes32();

    await expect(privacyGroup.transition(tx1ID, [], [], stateSet1, endorsements1)).to.be.ok;

    const { signatures: endorsements2 } =
      await newTransitionEIP712(privacyGroup, inputs2, [], stateSet2, endorsers)
    const tx2ID = randBytes32();

    await expect(privacyGroup.transition(tx2ID, inputs2, [], stateSet2, endorsements2)).to.
      revertedWithCustomError(privacyGroup, "PenteOutputAlreadyUnspent").withArgs(stateSet1[0]);

  });

});
