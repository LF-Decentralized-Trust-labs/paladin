import PaladinClient, {
  INotoDomainReceipt,
  IPreparedTransaction,
  NotoFactory,
  PenteFactory,
  ZetoFactory,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { ethers } from "ethers";
import { checkDeploy, checkReceipt } from "paladin-example-common";
import { newAtomFactory } from "./helpers/atom";
import { newERC20Tracker } from "./helpers/erc20tracker";

const logger = console;

const paladin1 = new PaladinClient({
  url: "http://127.0.0.1:31548",
});
const paladin2 = new PaladinClient({
  url: "http://127.0.0.1:31648",
});
const paladin3 = new PaladinClient({
  url: "http://127.0.0.1:31748",
});

// TODO: eliminate the need for this call
async function encodeZetoTransfer(preparedCashTransfer: IPreparedTransaction) {
  const zetoTransferAbi = await paladin3.getStoredABI(
    preparedCashTransfer.transaction.abiReference ?? ""
  );
  return new ethers.Interface(zetoTransferAbi.abi).encodeFunctionData(
    "transferLocked",
    [
      preparedCashTransfer.transaction.data.inputs,
      preparedCashTransfer.transaction.data.outputs,
      preparedCashTransfer.transaction.data.proof,
      preparedCashTransfer.transaction.data.data,
    ]
  );
}

async function main(): Promise<boolean> {
  const [cashIssuer, assetIssuer] = paladin1.getVerifiers(
    "cashIssuer@node1",
    "assetIssuer@node1"
  );
  const [investor1] = paladin2.getVerifiers("investor1@node2");
  const [investor2] = paladin3.getVerifiers("investor2@node3");

  // Deploy the atom factory on the base ledger
  logger.log("Creating atom factory...");
  const atomFactory = await newAtomFactory(paladin1, cashIssuer);
  if (!checkDeploy(atomFactory)) return false;

  // Deploy a Zeto token to represent cash
  logger.log("Deploying Zeto cash token...");
  const zetoFactory = new ZetoFactory(paladin1, "zeto");
  const zetoCash = await zetoFactory.newZeto(cashIssuer, {
    tokenName: "Zeto_Anon",
  });
  if (!checkDeploy(zetoCash)) return false;

  // Create a Pente privacy group for the asset issuer only
  logger.log("Creating asset issuer privacy group...");
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerGroup = await penteFactory.newPrivacyGroup({
    members: [assetIssuer],
    evmVersion: "shanghai",
    externalCallsEnabled: true,
  });
  if (!checkDeploy(issuerGroup)) return false;

  // Deploy private tracker to the issuer privacy group
  logger.log("Creating private asset tracker...");
  const tracker = await newERC20Tracker(issuerGroup, assetIssuer, {
    name: "ASSET",
    symbol: "ASSET",
  });
  if (!checkDeploy(tracker)) return false;

  // Create a Noto token to represent an asset
  logger.log("Deploying Noto asset token...");
  const notoFactory = new NotoFactory(paladin1, "noto");
  const notoAsset = await notoFactory.newNoto(assetIssuer, {
    notary: assetIssuer,
    notaryMode: "hooks",
    options: {
      hooks: {
        privateGroup: issuerGroup,
        publicAddress: issuerGroup.address,
        privateAddress: tracker.address,
      },
    },
  });
  if (!checkDeploy(notoAsset)) return false;

  // Issue asset
  logger.log("Issuing asset to investor1...");
  let receipt = await notoAsset.mint(assetIssuer, {
    to: investor1,
    amount: 1000,
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  // Issue cash
  logger.log("Issuing cash to investor2...");
  receipt = await zetoCash.mint(cashIssuer, {
    mints: [
      {
        to: investor2,
        amount: 10000,
        data: "0x",
      },
    ],
  });
  if (!checkReceipt(receipt)) return false;

  // Lock the asset for the swap
  logger.log("Locking asset from investor1...");
  receipt = await notoAsset.using(paladin2).lock(investor1, {
    amount: 100,
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;
  receipt = await paladin2.getTransactionReceipt(receipt.id, true);

  let domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const lockId = domainReceipt?.lockInfo?.lockId;
  if (lockId === undefined) {
    logger.error("No lock ID found in domain receipt");
    return false;
  }

  // Prepare asset unlock operation
  logger.log("Preparing unlock to investor2...");
  receipt = await notoAsset.using(paladin2).prepareUnlock(investor1, {
    lockId,
    from: investor1,
    recipients: [{ to: investor2, amount: 100 }],
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;
  receipt = await paladin2.getTransactionReceipt(receipt.id, true);

  domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const assetUnlockParams = domainReceipt?.lockInfo?.unlockParams;
  const assetUnlockCall = domainReceipt?.lockInfo?.unlockCall;
  if (assetUnlockParams === undefined || assetUnlockCall === undefined) {
    logger.error("No unlock data found in domain receipt");
    return false;
  }

  // Prepare cash transfer
  logger.log("Locking cash amount from investor2...");
  const investor2Address = await investor2.address();
  receipt = await zetoCash.using(paladin3).lock(investor2, {
    amount: 10,
    delegate: investor2Address,
  });
  if (!checkReceipt(receipt)) return false;
  const lockedStates = await paladin3.getStateReceipt(receipt.id);
  let lockedStateId: string | undefined;
  lockedStates?.confirmed?.forEach((state) => {
    if (state.data["locked"]) {
      lockedStateId = state.id;
    }
  });
  if (lockedStateId === undefined) {
    logger.error("No locked state found in state receipt");
    return false;
  } else {
    logger.log(`Locked state ID: ${lockedStateId}`);
  }

  const txID = await zetoCash.using(paladin3).prepareTransferLocked(investor2, {
    lockedInputs: [lockedStateId],
    delegate: investor2.lookup,
    transfers: [
      {
        to: investor1,
        amount: 10,
        data: "0x",
      },
    ],
  });
  const preparedCashTransfer = await paladin3.pollForPreparedTransaction(
    txID,
    10000
  );
  if (!preparedCashTransfer) return false;

  const encodedCashTransfer = await encodeZetoTransfer(preparedCashTransfer);

  // Create an atom for the swap
  logger.log("Creating atom...");
  const atom = await atomFactory.create(cashIssuer, [
    {
      contractAddress: notoAsset.address,
      callData: assetUnlockCall,
    },
    {
      contractAddress: zetoCash.address,
      callData: encodedCashTransfer,
    },
  ]);
  if (!checkDeploy(atom)) return false;

  // Approve asset unlock operation
  logger.log("Approving asset leg...");
  receipt = await notoAsset.using(paladin2).delegateLock(investor1, {
    lockId,
    unlock: assetUnlockParams,
    delegate: atom.address,
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  // Approve cash transfer operation
  logger.log("Approving cash leg...");
  receipt = await zetoCash.using(paladin3).delegateLock(investor2, {
    utxos: [lockedStateId],
    delegate: atom.address,
  });
  if (!checkReceipt(receipt)) return false;

  // execute the swap
  logger.log("Performing swap...");
  receipt = await atom.using(paladin3).execute(investor2);
  if (!checkReceipt(receipt)) return false;

  return true;
}

if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      console.error("Exiting with uncaught error");
      console.error(err);
      process.exit(1);
    });
}
