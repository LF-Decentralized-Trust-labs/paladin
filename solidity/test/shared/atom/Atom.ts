import { expect } from "chai";
import { ZeroAddress } from "ethers";
import { ethers } from "hardhat";
import { Atom, Noto } from "../../../typechain-types";
import {
  deployNotoInstance,
  fakeTXO,
  newUnlockHash,
  randomBytes32,
} from "../../domains/noto/util";

describe("Atom", function () {
  it("atomic operation with 2 encoded calls", async function () {
    const [notary1, notary2, anybody1, anybody2] = await ethers.getSigners();

    const NotoFactory = await ethers.getContractFactory("NotoFactory");
    const notoFactory = await NotoFactory.deploy();

    const Noto = await ethers.getContractFactory("Noto");
    const AtomFactory = await ethers.getContractFactory("AtomFactory");
    const Atom = await ethers.getContractFactory("Atom");
    const ERC20Simple = await ethers.getContractFactory("ERC20Simple");

    // Deploy two contracts
    const noto = Noto.attach(
      await deployNotoInstance(notoFactory, notary1.address)
    ) as Noto;
    const erc20 = await ERC20Simple.connect(notary2).deploy("Token", "TOK");

    // Bring TXOs and tokens into being
    const lockId = randomBytes32();
    const [f1txo1, f1txo2] = [fakeTXO(), fakeTXO()];
    await noto.connect(notary1).lock(
      randomBytes32(),
      lockId,
      {
        inputs: [],
        outputs: [],
        lockedOutputs: [f1txo1, f1txo2],
      },
      ZeroAddress,
      "0x",
      "0x",
      "0x"
    );
    await erc20.mint(notary2, 1000);

    // Encode two function calls
    const [f1txo3, f1txo4] = [fakeTXO(), fakeTXO()];
    const f1TxData = randomBytes32();
    const unlockHash = await newUnlockHash(
      noto,
      [f1txo1, f1txo2],
      [],
      [f1txo3, f1txo4],
      f1TxData
    );
    const encoded1 = noto.interface.encodeFunctionData("transferLocked", [
      randomBytes32(),
      lockId,
      [f1txo1, f1txo2],
      [],
      [f1txo3, f1txo4],
      "0x",
      f1TxData,
    ]);
    const encoded2 = erc20.interface.encodeFunctionData("transferFrom", [
      notary2.address,
      notary1.address,
      1000,
    ]);

    // Deploy the delegation contract
    const atomFactory = await AtomFactory.connect(anybody1).deploy();
    const atomFactoryInvoke = await atomFactory.connect(anybody1).create([
      {
        contractAddress: noto,
        callData: encoded1,
      },
      {
        contractAddress: erc20,
        callData: encoded2,
      },
    ]);
    const createAtom = await atomFactoryInvoke.wait();
    const createAtomEvent = createAtom?.logs
      .map((l) => AtomFactory.interface.parseLog(l))
      .find((l) => l?.name === "AtomDeployed");
    const atomAddr = createAtomEvent?.args.addr;

    // Do the delegation/approval transactions
    const options = ethers.AbiCoder.defaultAbiCoder().encode(
      ["tuple(bytes32)"],
      [[unlockHash]]
    );
    await noto
      .connect(notary1)
      .setLockOptions(randomBytes32(), lockId, [f1txo1, f1txo2], options, "0x", "0x");
    await noto.connect(notary1).delegateLock(randomBytes32(), lockId, atomAddr, "0x", "0x");
    await erc20.approve(atomAddr, 1000);

    // Run the atomic op (anyone can initiate)
    const atom = Atom.connect(anybody2).attach(atomAddr) as Atom;
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

    const NotoFactory = await ethers.getContractFactory("NotoFactory");
    const notoFactory = await NotoFactory.deploy();

    const Noto = await ethers.getContractFactory("Noto");
    const AtomFactory = await ethers.getContractFactory("AtomFactory");
    const Atom = await ethers.getContractFactory("Atom");

    // Deploy noto contract
    const noto = Noto.attach(
      await deployNotoInstance(notoFactory, notary1.address)
    ) as Noto;

    // Fake up a delegation
    const lockId = randomBytes32();
    const [f1txo1, f1txo2] = [fakeTXO(), fakeTXO()];
    const [f1txo3, f1txo4] = [fakeTXO(), fakeTXO()];
    const f1TxData = randomBytes32();

    const encoded1 = noto.interface.encodeFunctionData("transferLocked", [
      randomBytes32(),
      lockId,
      [f1txo1, f1txo2],
      [],
      [f1txo3, f1txo4],
      randomBytes32(),
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
      .to.be.revertedWithCustomError(Noto, "NotoNotNotary")
      .withArgs(mcAddr);
  });
});
