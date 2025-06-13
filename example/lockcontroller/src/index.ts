import PaladinClient, {
  INotoDomainReceipt,
  newGroupSalt,
  NotoFactory,
  PenteFactory,
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { ethers } from "ethers";
import erc20Json from "./abis/NotoERC20.json";
import { newERC20Tracker } from "./helpers/erc20tracker";
import { newLockController } from "./helpers/lockcontroller";
import { checkDeploy, checkReceipt } from "./util";

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

async function main(): Promise<boolean> {
  const [cashIssuer, assetIssuer] = paladin1.getVerifiers(
    "cashIssuer@node1",
    "assetIssuer@node1"
  );
  const [investor1, investor2] = paladin2.getVerifiers(
    "investor1@node2",
    "investor2@node2"
  );

  // Deploy the lock controller on the base ledger
  logger.log("Creating lock controller...");
  const lockController = await newLockController(paladin1, cashIssuer);
  if (!checkDeploy(lockController)) return false;

  // Create a Pente privacy group for the asset issuer only
  logger.log("Creating asset issuer privacy group...");
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerGroup = await penteFactory.newPrivacyGroup(assetIssuer, {
    group: {
      salt: newGroupSalt(),
      members: [assetIssuer],
    },
    evmVersion: "shanghai",
    endorsementType: "group_scoped_identities",
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
        privateGroup: issuerGroup.group,
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

  // Lock the asset, creating a new ERC20 via the LockController
  logger.log("Locking asset from investor1...");
  const abiCoder = ethers.AbiCoder.defaultAbiCoder();
  receipt = await notoAsset.using(paladin2).lock(investor1, {
    amount: 100,
    data: abiCoder.encode(
      ["address", "address"],
      [lockController.address, await investor1.address()]
    ),
  });
  if (!checkReceipt(receipt)) return false;
  receipt = await paladin2.getTransactionReceipt(receipt.id, true);

  let domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const lockId = domainReceipt?.lockInfo?.lockId;
  if (lockId === undefined) {
    logger.error("No lock ID found in domain receipt");
    return false;
  }

  const lockedTokenResult = await lockController
    .using(paladin2)
    .getToken(investor1, lockId);
  const lockedTokenAddress = lockedTokenResult?.[0];
  if (lockedTokenAddress === undefined) {
    logger.error("No locked token address found");
    return false;
  }

  // Transfer the locked ERC20
  logger.log("Transferring locked ERC20 to investor2...");
  const txID = await paladin2.sendTransaction({
    type: TransactionType.PUBLIC,
    from: investor1.lookup,
    to: lockedTokenAddress,
    abi: erc20Json.abi,
    function: "transfer",
    data: { to: await investor2.address(), value: 100 },
  });
  receipt = await paladin2.pollForReceipt(txID, 10000);
  if (!checkReceipt(receipt)) return false;

  // Unlock the asset, exchanging ERC20 tokens for Noto
  // Note: currenly only works for users on the same node as the locking user,
  // as other nodes won't have the locked state details
  logger.log("Unlocking asset from investor2...");
  receipt = await notoAsset.using(paladin2).unlock(investor2, {
    lockId,
    from: investor1,
    recipients: [{ to: investor2, amount: 100 }],
    data: abiCoder.encode(["address"], [await investor2.address()]),
  });
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
