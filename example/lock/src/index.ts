import PaladinClient, {
  newGroupSalt,
  NotoFactory,
  PenteFactory,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { newERC20Tracker } from "./helpers/erc20tracker";
import { checkDeploy, checkReceipt } from "./util";
import { randomBytes } from "crypto";

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
  const [cashIssuer] = paladin1.getVerifiers("cashIssuer@node1");
  const [investor1] = paladin2.getVerifiers("investor1@node2");
  const [investor2] = paladin3.getVerifiers("investor2@node3");

  // Create a Pente privacy group for the issuer only
  logger.log("Creating issuer privacy group...");
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerGroup = await penteFactory.newPrivacyGroup(cashIssuer, {
    group: {
      salt: newGroupSalt(),
      members: [cashIssuer],
    },
    evmVersion: "shanghai",
    endorsementType: "group_scoped_identities",
    externalCallsEnabled: true,
  });
  if (!checkDeploy(issuerGroup)) return false;

  // Deploy private tracker to the issuer privacy group
  logger.log("Creating private tracker...");
  const tracker = await newERC20Tracker(issuerGroup, cashIssuer, {
    name: "CASH",
    symbol: "CASH",
  });
  if (!checkDeploy(tracker)) return false;

  // Create a Noto token to represent cash
  logger.log("Deploying Noto cash token...");
  const notoFactory = new NotoFactory(paladin1, "noto");
  const notoCash = await notoFactory.newNoto(cashIssuer, {
    notary: cashIssuer,
    notaryMode: "hooks",
    options: {
      hooks: {
        privateGroup: issuerGroup.group,
        publicAddress: issuerGroup.address,
        privateAddress: tracker.address,
      },
    },
  });
  if (!checkDeploy(notoCash)) return false;

  // Issue some cash
  logger.log("Issuing cash to investor1...");
  let receipt = await notoCash.mint(cashIssuer, {
    to: investor1,
    amount: 1000,
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  // Lock some tokens
  logger.log("Locking cash from investor1...");
  const lockId = randomBytes(32).toString("hex");
  receipt = await notoCash.using(paladin2).lock(investor1, {
    lockId,
    amount: 100,
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  // Prepare unlock operation
  logger.log("Preparing unlock to investor2...");
  receipt = await notoCash.using(paladin2).prepareUnlock(investor1, {
    lockId,
    from: investor1,
    to: [investor2],
    amounts: [100],
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;
  receipt = await paladin2.getTransactionReceipt(receipt.id, true);

  const unlockInputs = receipt?.states?.read?.map((s) => s.id);
  const unlockOutputs = receipt?.states?.info
    ?.filter((s) => s.data["amount"] !== undefined)
    .map((s) => s.id);

  // Approve unlock operation
  logger.log("Delegating lock to investor2...");
  receipt = await notoCash.using(paladin2).delegateLock(investor1, {
    lockId,
    delegate: await investor2.address(),
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  // Unlock the tokens
  logger.log("Unlocking cash...");
  receipt = await notoCash.using(paladin3).unlockWithApproval(investor2, {
    lockId,
    lockedInputs: unlockInputs ?? [],
    lockedOutputs: [],
    outputs: unlockOutputs ?? [],
    data: "0x",
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
