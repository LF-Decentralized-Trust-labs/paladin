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
  const [investor] = paladin3.getVerifiers("investor@node2");

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
    hooks: {
      privateGroup: issuerGroup.group,
      publicAddress: issuerGroup.address,
      privateAddress: tracker.address,
    },
    restrictMint: true,
    allowBurn: true,
  });
  if (!checkDeploy(notoCash)) return false;

  // Issue some cash
  logger.log("Issuing cash...");
  let receipt = await notoCash.mint(cashIssuer, {
    to: investor,
    amount: 1000,
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  // Lock some tokens
  logger.log("Locking cash...");
  const lockId = randomBytes(32).toString("hex");
  const investorAdddress = await investor.address();
  receipt = await notoCash.using(paladin2).lock(investor, {
    id: lockId,
    // delegate: investorAdddress,
    amount: 100,
    // recipients: [
    //   {
    //     ref: 0,
    //     recipient: investorAdddress,
    //   },
    // ],
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  const schemas = await paladin2.listSchemas("noto");
  const lockSchema = schemas.find((schema) =>
    schema.signature.startsWith("type=NotoLockedCoin")
  );
  if (!lockSchema) {
    logger.error("Failed to find lock schema");
    return false;
  }
  const locks = await paladin2.queryContractStates(
    "noto",
    notoCash.address,
    lockSchema.id,
    {},
    "available"
  );
  if (locks.length !== 1) {
    logger.error("Failed to find lock state");
    return false;
  }
  const lock = locks[0];

  // Unlock the tokens
  logger.log("Unlocking cash...");
  receipt = await notoCash.using(paladin2).unlock(investor, {
    locked: lock.id,
    outcome: 0,
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
