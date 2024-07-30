import { expect } from "chai";
import { ContractTransactionReceipt, ZeroAddress } from "ethers";
import { ethers } from "hardhat";
import { formatPrivKeyForBabyJub } from "maci-crypto";
import { groth16 } from "snarkjs";
import { encodeProof, loadCircuit } from "zeto-js";
import { MultiCall } from "../typechain-types";
import { fakeTXO, newTransferHash, randomBytes32 } from "./noto/Noto";
import { UTXO, User, loadProvingKeys, newUTXO, newUser } from "./utils";

const ZERO_PUBKEY = [0, 0];

enum OperationType {
  EncodedCall = 0,
  ERC20Transfer,
  ERC721Transfer,
  NotoTransfer,
  ZetoTransfer,
}

const newOperation = (
  op: Partial<MultiCall.OperationInputStruct> &
    Pick<MultiCall.OperationInputStruct, "opType" | "contractAddress">
): MultiCall.OperationInputStruct => {
  return {
    fromAddress: ethers.ZeroAddress,
    toAddress: ethers.ZeroAddress,
    value: 0,
    tokenIndex: 0,
    inputs: [],
    outputs: [],
    signature: "0x",
    data: "0x",
    proof: {
      pA: [0, 0],
      pB: [
        [0, 0],
        [0, 0],
      ],
      pC: [0, 0],
    },
    ...op,
  };
};

describe("MultiCall", function () {
  it("atomic operation with 2 encoded calls", async function () {
    const [notary1, notary2, anybody1, anybody2] = await ethers.getSigners();

    const Noto = await ethers.getContractFactory("Noto");
    const MultiCallFactory = await ethers.getContractFactory(
      "MultiCallFactory"
    );
    const MultiCall = await ethers.getContractFactory("MultiCall");
    const ERC20Simple = await ethers.getContractFactory("ERC20Simple");

    // Deploy two contracts
    const noto = await Noto.connect(notary1).deploy(notary1.address);
    const erc20 = await ERC20Simple.connect(notary2).deploy("Token", "TOK");

    // Bring TXOs and tokens into being
    const [f1txo1, f1txo2] = [fakeTXO(), fakeTXO()];
    await noto
      .connect(notary1)
      .transfer([], [f1txo1, f1txo2], "0x", randomBytes32());

    await erc20.mint(notary2, 1000);

    // Encode two function calls
    const [f1txo3, f1txo4] = [fakeTXO(), fakeTXO()];
    const f1TxData = randomBytes32();
    const multiTXF1Part = await newTransferHash(
      noto,
      [f1txo1, f1txo2],
      [f1txo3, f1txo4],
      f1TxData
    );
    const encoded1 = noto.interface.encodeFunctionData("approvedTransfer", [
      [f1txo1, f1txo2],
      [f1txo3, f1txo4],
      f1TxData,
    ]);
    const encoded2 = erc20.interface.encodeFunctionData("transferFrom", [
      notary2.address,
      notary1.address,
      1000,
    ]);

    // Deploy the delegation contract
    const multiCallFactory = await MultiCallFactory.connect(anybody1).deploy();
    const mcFactoryInvoke = await multiCallFactory.connect(anybody1).create([
      newOperation({
        opType: OperationType.EncodedCall,
        contractAddress: noto,
        data: encoded1,
      }),
      newOperation({
        opType: OperationType.EncodedCall,
        contractAddress: erc20,
        data: encoded2,
      }),
    ]);
    const createMF = await mcFactoryInvoke.wait();
    const createMFEvent = createMF?.logs
      .map((l) => MultiCallFactory.interface.parseLog(l))
      .find((l) => l?.name === "MultiCallDeployed");
    const mcAddr = createMFEvent?.args.addr;

    // Do the delegation/approval transactions
    const f1tx = await noto
      .connect(notary1)
      .approve(mcAddr, multiTXF1Part.hash, "0x");
    const delegateResult1: ContractTransactionReceipt | null =
      await f1tx.wait();
    const delegateEvent1 = noto.interface.parseLog(
      delegateResult1?.logs[0] as any
    )!.args;
    expect(delegateEvent1.delegate).to.equal(mcAddr);
    expect(delegateEvent1.txhash).to.equal(multiTXF1Part.hash);
    await erc20.approve(mcAddr, 1000);

    // Run the atomic op (anyone can initiate)
    const multiCall = MultiCall.connect(anybody2).attach(mcAddr) as MultiCall;
    await multiCall.execute();

    // Now we should find the final TXOs/tokens in both contracts in the right states
    expect(await noto.isUnspent(f1txo1)).to.equal(false);
    expect(await noto.isUnspent(f1txo2)).to.equal(false);
    expect(await noto.isUnspent(f1txo3)).to.equal(true);
    expect(await noto.isUnspent(f1txo4)).to.equal(true);
    expect(await erc20.balanceOf(notary2)).to.equal(0);
    expect(await erc20.balanceOf(notary1)).to.equal(1000);
  });

  it("atomic operation with ERC20 transfer and Noto transfer", async function () {
    const [notary1, notary2, anybody1, anybody2] = await ethers.getSigners();

    const Noto = await ethers.getContractFactory("Noto");
    const MultiCallFactory = await ethers.getContractFactory(
      "MultiCallFactory"
    );
    const MultiCall = await ethers.getContractFactory("MultiCall");
    const ERC20Simple = await ethers.getContractFactory("ERC20Simple");

    // Deploy two contracts
    const noto = await Noto.connect(notary1).deploy(notary1.address);
    const erc20 = await ERC20Simple.connect(notary2).deploy("Token", "TOK");

    // Bring TXOs and tokens into being
    const [f1txo1, f1txo2] = [fakeTXO(), fakeTXO()];
    await noto
      .connect(notary1)
      .transfer([], [f1txo1, f1txo2], "0x", randomBytes32());

    await erc20.mint(notary2, 1000);

    const [f1txo3, f1txo4] = [fakeTXO(), fakeTXO()];
    const f1TxData = randomBytes32();
    const multiTXF1Part = await newTransferHash(
      noto,
      [f1txo1, f1txo2],
      [f1txo3, f1txo4],
      f1TxData
    );

    // Deploy the delegation contract
    const multiCallFactory = await MultiCallFactory.connect(anybody1).deploy();
    const mcFactoryInvoke = await multiCallFactory.connect(anybody1).create([
      newOperation({
        opType: OperationType.NotoTransfer,
        contractAddress: noto,
        inputs: [f1txo1, f1txo2],
        outputs: [f1txo3, f1txo4],
        data: f1TxData,
      }),
      newOperation({
        opType: OperationType.ERC20Transfer,
        contractAddress: erc20,
        fromAddress: notary2.address,
        toAddress: notary1.address,
        value: 1000,
      }),
    ]);
    const createMF = await mcFactoryInvoke.wait();
    const createMFEvent = createMF?.logs
      .map((l) => MultiCallFactory.interface.parseLog(l))
      .find((l) => l?.name === "MultiCallDeployed");
    const mcAddr = createMFEvent?.args.addr;

    // Do the delegation/approval transactions
    const f1tx = await noto
      .connect(notary1)
      .approve(mcAddr, multiTXF1Part.hash, "0x");
    const delegateResult1: ContractTransactionReceipt | null =
      await f1tx.wait();
    const delegateEvent1 = noto.interface.parseLog(
      delegateResult1?.logs[0] as any
    )!.args;
    expect(delegateEvent1.delegate).to.equal(mcAddr);
    expect(delegateEvent1.txhash).to.equal(multiTXF1Part.hash);
    await erc20.approve(mcAddr, 1000);

    // Run the atomic op (anyone can initiate)
    const multiCall = MultiCall.connect(anybody2).attach(mcAddr) as MultiCall;
    await multiCall.execute();

    // Now we should find the final TXOs/tokens in both contracts in the right states
    expect(await noto.isUnspent(f1txo1)).to.equal(false);
    expect(await noto.isUnspent(f1txo2)).to.equal(false);
    expect(await noto.isUnspent(f1txo3)).to.equal(true);
    expect(await noto.isUnspent(f1txo4)).to.equal(true);
    expect(await erc20.balanceOf(notary2)).to.equal(0);
    expect(await erc20.balanceOf(notary1)).to.equal(1000);
  });

  it("atomic operation with Noto transfer and Zeto transfer", async function () {
    const [notary1, anybody1, anybody2] = await ethers.getSigners();

    const Noto = await ethers.getContractFactory("Noto");
    const MultiCallFactory = await ethers.getContractFactory(
      "MultiCallFactory"
    );
    const MultiCall = await ethers.getContractFactory("MultiCall");

    const Commonlib = await ethers.getContractFactory("Commonlib");
    const commonlib = await Commonlib.deploy();
    const Registry = await ethers.getContractFactory("Registry");
    const Verifier = await ethers.getContractFactory("Groth16Verifier_Anon");
    const Zeto = await ethers.getContractFactory("Zeto_Anon", {
      libraries: { Commonlib: commonlib },
    });

    const registry = await Registry.deploy();
    const verifier = await Verifier.deploy();

    // Populate registry
    const Alice = await newUser(anybody1);
    const Bob = await newUser(anybody2);
    await registry
      .connect(notary1)
      .register(Alice.ethAddress, Alice.babyJubPublicKey);
    await registry
      .connect(notary1)
      .register(Bob.ethAddress, Bob.babyJubPublicKey);

    // Deploy two contracts
    const noto = await Noto.connect(notary1).deploy(notary1.address);
    const zeto = await Zeto.deploy(verifier, registry);

    // Bring TXOs into being
    const [f1txo1, f1txo2] = [fakeTXO(), fakeTXO()];
    await noto
      .connect(notary1)
      .transfer([], [f1txo1, f1txo2], "0x", randomBytes32());

    const [f2txo1, f2txo2] = [newUTXO(10, Alice), newUTXO(20, Alice)];
    await zeto.connect(notary1).mint([f2txo1.hash, f2txo2.hash]);

    const [f1txo3, f1txo4] = [fakeTXO(), fakeTXO()];
    const f1TxData = randomBytes32();
    const multiTXF1Part = await newTransferHash(
      noto,
      [f1txo1, f1txo2],
      [f1txo3, f1txo4],
      f1TxData
    );

    const f2txo3 = newUTXO(25, Bob);
    const f2txo4 = newUTXO(5, Alice, f2txo3.salt);

    // Prepare zkp
    const circuit = await loadCircuit("anon");
    const { provingKeyFile: provingKey } = loadProvingKeys("anon");
    const result = await prepareProof(
      circuit,
      provingKey,
      Alice,
      [f2txo1, f2txo2],
      [f2txo3, f2txo4],
      [Bob, Alice]
    );

    // Deploy the delegation contract
    const multiCallFactory = await MultiCallFactory.connect(anybody1).deploy();
    const mcFactoryInvoke = await multiCallFactory.connect(anybody1).create([
      newOperation({
        opType: OperationType.NotoTransfer,
        contractAddress: noto,
        inputs: [f1txo1, f1txo2],
        outputs: [f1txo3, f1txo4],
        data: f1TxData,
      }),
      newOperation({
        opType: OperationType.ZetoTransfer,
        contractAddress: zeto,
        inputs: [f2txo1.hash, f2txo2.hash],
        outputs: [f2txo3.hash, f2txo4.hash],
        proof: result.encodedProof,
      }),
    ]);
    const createMF = await mcFactoryInvoke.wait();
    const createMFEvent = createMF?.logs
      .map((l) => MultiCallFactory.interface.parseLog(l))
      .find((l) => l?.name === "MultiCallDeployed");
    const mcAddr = createMFEvent?.args.addr;

    // Do the delegation/approval transactions
    const f1tx = await noto
      .connect(notary1)
      .approve(mcAddr, multiTXF1Part.hash, "0x");
    const delegateResult1: ContractTransactionReceipt | null =
      await f1tx.wait();
    const delegateEvent1 = noto.interface.parseLog(
      delegateResult1?.logs[0] as any
    )!.args;
    expect(delegateEvent1.delegate).to.equal(mcAddr);
    expect(delegateEvent1.txhash).to.equal(multiTXF1Part.hash);
    // no delegation for zkp?

    // Run the atomic op (anyone can initiate)
    const multiCall = MultiCall.connect(anybody2).attach(mcAddr) as MultiCall;
    await multiCall.execute();

    // Now we should find the final TXOs in both contracts in the right states
    expect(await noto.isUnspent(f1txo1)).to.equal(false);
    expect(await noto.isUnspent(f1txo2)).to.equal(false);
    expect(await noto.isUnspent(f1txo3)).to.equal(true);
    expect(await noto.isUnspent(f1txo4)).to.equal(true);
    expect(await zeto.spent(f2txo1.hash)).to.equal(true);
    expect(await zeto.spent(f2txo2.hash)).to.equal(true);
    expect(await zeto.spent(f2txo3.hash)).to.equal(false);
    expect(await zeto.spent(f2txo4.hash)).to.equal(false);
  });

  it("revert propagation", async function () {
    const [notary1, anybody1, anybody2] = await ethers.getSigners();

    const Noto = await ethers.getContractFactory("Noto");
    const MultiCallFactory = await ethers.getContractFactory(
      "MultiCallFactory"
    );
    const MultiCall = await ethers.getContractFactory("MultiCall");

    // Deploy noto contract
    const noto = await Noto.connect(notary1).deploy(notary1.address);

    // Fake up a delegation
    const [f1txo1, f1txo2] = [fakeTXO(), fakeTXO()];
    const [f1txo3, f1txo4] = [fakeTXO(), fakeTXO()];
    const f1TxData = randomBytes32();
    const multiTXF1Part = await newTransferHash(
      noto,
      [f1txo1, f1txo2],
      [f1txo3, f1txo4],
      f1TxData
    );

    const encoded1 = noto.interface.encodeFunctionData("approvedTransfer", [
      [f1txo1, f1txo2],
      [f1txo3, f1txo4],
      f1TxData,
    ]);

    // Deploy the delegation contract
    const multiCallFactory = await MultiCallFactory.connect(anybody1).deploy();
    const mcFactoryInvoke = await multiCallFactory.connect(anybody1).create([
      newOperation({
        opType: OperationType.EncodedCall,
        contractAddress: noto,
        data: encoded1,
      }),
    ]);
    const createMF = await mcFactoryInvoke.wait();
    const createMFEvent = createMF?.logs
      .map((l) => MultiCallFactory.interface.parseLog(l))
      .find((l) => l?.name === "MultiCallDeployed");
    const mcAddr = createMFEvent?.args.addr;

    // Run the atomic op (will revert because delegation was never actually created)
    const multiCall = MultiCall.connect(anybody2).attach(mcAddr) as MultiCall;
    await expect(multiCall.execute())
      .to.be.revertedWithCustomError(Noto, "NotoInvalidDelegate")
      .withArgs(multiTXF1Part.hash, ZeroAddress, mcAddr);
  });
});

async function prepareProof(
  circuit: any,
  provingKey: any,
  signer: User,
  inputs: UTXO[],
  outputs: UTXO[],
  owners: User[]
) {
  const inputCommitments = inputs.map((input) => input.hash);
  const inputValues = inputs.map((input) => BigInt(input.value || 0n));
  const inputSalts = inputs.map((input) => input.salt || 0n);
  const outputCommitments = outputs.map((output) => output.hash);
  const outputValues = outputs.map((output) => BigInt(output.value || 0n));
  const outputSalts = outputs.map((o) => o.salt || 0n);
  const outputOwnerPublicKeys = owners.map(
    (owner) => owner.babyJubPublicKey || ZERO_PUBKEY
  );
  const senderPrivateKey = formatPrivKeyForBabyJub(signer.babyJubPrivateKey);

  const witness = await circuit.calculateWTNSBin(
    {
      inputCommitments,
      inputValues,
      inputSalts,
      outputCommitments,
      outputValues,
      outputSalts,
      outputOwnerPublicKeys,
      senderPrivateKey,
    },
    true
  );

  const { proof } = await groth16.prove(provingKey, witness);
  const encodedProof = encodeProof(proof);
  return {
    inputCommitments,
    outputCommitments,
    encodedProof,
  };
}
