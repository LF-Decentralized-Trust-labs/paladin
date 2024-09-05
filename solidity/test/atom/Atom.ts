import { expect } from "chai";
import { ContractTransactionReceipt, ZeroAddress } from "ethers";
import { ethers } from "hardhat";
import { Atom } from "../typechain-types";
import { fakeTXO, newTransferHash, randomBytes32 } from "../noto/Noto";

describe("Atom", function () {
  it("atomic operation with 2 encoded calls", async function () {
    const [notary1, notary2, anybody1, anybody2] = await ethers.getSigners();

    const Noto = await ethers.getContractFactory("Noto");
    const AtomFactory = await ethers.getContractFactory("AtomFactory");
    const Atom = await ethers.getContractFactory("Atom");
    const ERC20Simple = await ethers.getContractFactory("ERC20Simple");

    // Deploy two contracts
    const noto = await Noto.connect(notary1).deploy(
      randomBytes32(),
      anybody1.address,
      notary1.address,
      "0x"
    );
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
    const atomFactory = await AtomFactory.connect(anybody1).deploy();
    const mcFactoryInvoke = await atomFactory.connect(anybody1).create([
      {
        contractAddress: noto,
        callData: encoded1,
      },
      {
        contractAddress: erc20,
        callData: encoded2,
      },
    ]);
    const createMF = await mcFactoryInvoke.wait();
    const createMFEvent = createMF?.logs
      .map((l) => AtomFactory.interface.parseLog(l))
      .find((l) => l?.name === "AtomDeployed");
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
    const atom = Atom.connect(anybody2).attach(mcAddr) as Atom;
    await atom.execute();

    // Now we should find the final TXOs/tokens in both contracts in the right states
    expect(await noto.isUnspent(f1txo1)).to.equal(false);
    expect(await noto.isUnspent(f1txo2)).to.equal(false);
    expect(await noto.isUnspent(f1txo3)).to.equal(true);
    expect(await noto.isUnspent(f1txo4)).to.equal(true);
    expect(await erc20.balanceOf(notary2)).to.equal(0);
    expect(await erc20.balanceOf(notary1)).to.equal(1000);
  });

  it("revert propagation", async function () {
    const [notary1, anybody1, anybody2] = await ethers.getSigners();

    const Noto = await ethers.getContractFactory("Noto");
    const AtomFactory = await ethers.getContractFactory("AtomFactory");
    const Atom = await ethers.getContractFactory("Atom");

    // Deploy noto contract
    const noto = await Noto.connect(notary1).deploy(
      randomBytes32(),
      anybody1.address,
      notary1.address,
      "0x"
    );

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
    const atomFactory = await AtomFactory.connect(anybody1).deploy();
    const mcFactoryInvoke = await atomFactory.connect(anybody1).create([
      {
        contractAddress: noto,
        callData: encoded1,
      },
    ]);
    const createMF = await mcFactoryInvoke.wait();
    const createMFEvent = createMF?.logs
      .map((l) => AtomFactory.interface.parseLog(l))
      .find((l) => l?.name === "AtomDeployed");
    const mcAddr = createMFEvent?.args.addr;

    // Run the atomic op (will revert because delegation was never actually created)
    const atom = Atom.connect(anybody2).attach(mcAddr) as Atom;
    await expect(atom.execute())
      .to.be.revertedWithCustomError(Noto, "NotoInvalidDelegate")
      .withArgs(multiTXF1Part.hash, ZeroAddress, mcAddr);
  });
});
