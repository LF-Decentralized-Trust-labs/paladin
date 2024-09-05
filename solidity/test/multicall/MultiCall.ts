import { expect } from "chai";
import { ContractTransactionReceipt, ZeroAddress } from "ethers";
import { ethers } from "hardhat";
import { MultiCall } from "../typechain-types";
import { fakeTXO, newTransferHash, randomBytes32 } from "../noto/Noto";

enum OperationType {
  EncodedCall = 0,
}

const newOperation = (
  op: Partial<MultiCall.OperationInputStruct> &
    Pick<MultiCall.OperationInputStruct, "opType" | "contractAddress">
): MultiCall.OperationInputStruct => {
  return {
    data: "0x",
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

  it("revert propagation", async function () {
    const [notary1, anybody1, anybody2] = await ethers.getSigners();

    const Noto = await ethers.getContractFactory("Noto");
    const MultiCallFactory = await ethers.getContractFactory(
      "MultiCallFactory"
    );
    const MultiCall = await ethers.getContractFactory("MultiCall");

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
