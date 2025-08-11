import PaladinClient, {
  INotoDomainReceipt,
  NotoBalanceOfResult,
  NotoFactory,
  PenteFactory,
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { checkDeploy, checkReceipt } from "paladin-example-common";
import atomJson from "./abis/Atom.json";
import atomFactoryJson from "./abis/AtomFactory.json";
import bondTrackerPublicJson from "./abis/BondTrackerPublic.json";
import { newBondSubscription } from "./helpers/bondsubscription";
import { newBondTracker } from "./helpers/bondtracker";
import * as fs from 'fs';
import * as path from 'path';
import { ContractData } from "./verify-deployed";

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
  const [cashIssuer, bondIssuer] = paladin1.getVerifiers(
    "cashIssuer@node1",
    "bondIssuer@node1"
  );
  const [bondCustodian] = paladin2.getVerifiers("bondCustodian@node2");
  const [investor] = paladin3.getVerifiers("investor@node3");

  // Create a Noto token to represent cash
  logger.log("Deploying Noto cash token...");
  const notoFactory = new NotoFactory(paladin1, "noto");
  const notoCash = await notoFactory
    .newNoto(cashIssuer, {
      notary: cashIssuer,
      notaryMode: "basic",
    })
    .waitForDeploy();
  if (!checkDeploy(notoCash)) return false;

  // Issue some cash
  logger.log("Issuing cash...");
  let receipt = await notoCash
    .mint(cashIssuer, {
      to: investor,
      amount: 100000,
      data: "0x",
    })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;

  let balanceInvestor = await notoCash.balanceOf(cashIssuer, {
    account: investor.lookup,
  });
  logger.log(
    `(NotoCash) Investor State: ${balanceInvestor.totalBalance} units of cash, ${balanceInvestor.totalStates} states, overflow: ${balanceInvestor.overflow}`
  );

  // Create a Pente privacy group between the bond issuer and bond custodian
  logger.log("Creating issuer+custodian privacy group...");
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerCustodianGroup = await penteFactory
    .newPrivacyGroup({
      members: [bondIssuer, bondCustodian],
      evmVersion: "shanghai",
      externalCallsEnabled: true,
    })
    .waitForDeploy();
  if (!checkDeploy(issuerCustodianGroup)) return false;

  // Deploy the public bond tracker on the base ledger (controlled by the privacy group)
  logger.log("Creating public bond tracker...");
  const issueDate = Math.floor(Date.now() / 1000);
  const maturityDate = issueDate + 60 * 60 * 24;
  let txID = await paladin1.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: bondTrackerPublicJson.abi,
    bytecode: bondTrackerPublicJson.bytecode,
    function: "",
    from: bondIssuer.lookup,
    data: {
      owner: issuerCustodianGroup.address,
      issueDate_: issueDate,
      maturityDate_: maturityDate,
      currencyToken_: notoCash.address,
      faceValue_: 1,
    },
  });
  receipt = await paladin1.pollForReceipt(txID, 10000);
  if (receipt?.contractAddress === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log(`Success! address: ${receipt.contractAddress}`);
  const bondTrackerPublicAddress = receipt.contractAddress;

  // Deploy private bond tracker to the issuer/custodian privacy group
  logger.log("Creating private bond tracker...");
  const bondTracker = await newBondTracker(issuerCustodianGroup, bondIssuer, {
    name: "BOND",
    symbol: "BOND",
    custodian: await bondCustodian.address(),
    publicTracker: bondTrackerPublicAddress,
  });
  if (!checkDeploy(bondTracker)) return false;

  // Deploy Noto token to represent bond
  logger.log("Deploying Noto bond token...");
  const notoBond = await notoFactory
    .newNoto(bondIssuer, {
      notary: bondCustodian,
      notaryMode: "hooks",
      options: {
        hooks: {
          privateGroup: issuerCustodianGroup,
          publicAddress: issuerCustodianGroup.address,
          privateAddress: bondTracker.address,
        },
      },
    })
    .waitForDeploy();
  if (!checkDeploy(notoBond)) return false;

  // Deploy the atom factory on the base ledger
  logger.log("Creating atom factory...");
  txID = await paladin1.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: atomFactoryJson.abi,
    bytecode: atomFactoryJson.bytecode,
    function: "",
    from: bondIssuer.lookup,
    data: {},
  });
  receipt = await paladin1.pollForReceipt(txID, 10000);
  if (receipt?.contractAddress === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log(`Success! address: ${receipt.contractAddress}`);
  const atomFactoryAddress = receipt.contractAddress;

  // Issue the bond to the custodian
  logger.log("Issuing bond...");
  receipt = await notoBond
    .mint(bondIssuer, {
      to: bondCustodian,
      amount: 1000,
      data: "0x",
    })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;
  let balanceCustodian = await notoBond.balanceOf(bondIssuer, {
    account: bondCustodian.lookup,
  });
  logger.log(
    `(NotoBond) Bond Custodian State: ${balanceCustodian.totalBalance} units of cash, ${balanceCustodian.totalStates} states, overflow: ${balanceCustodian.overflow}`
  );

  // Begin bond distribution to investors
  logger.log("Beginning distribution...");
  receipt = await bondTracker
    .using(paladin2)
    .beginDistribution(bondCustodian, {
      discountPrice: 1,
      minimumDenomination: 1,
    })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;

  // Add allowed investors
  const investorList = await bondTracker.investorList(bondIssuer);
  receipt = await investorList
    .using(paladin2)
    .addInvestor(bondCustodian, { addr: await investor.address() })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;

  // Create a Pente privacy group between the bond investor and bond custodian
  logger.log("Creating investor+custodian privacy group...");
  const investorCustodianGroup = await penteFactory
    .using(paladin3)
    .newPrivacyGroup({
      members: [investor, bondCustodian],
      evmVersion: "shanghai",
      externalCallsEnabled: true,
    })
    .waitForDeploy();
  if (investorCustodianGroup === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log(`Success! address: ${investorCustodianGroup.address}`);

  // Deploy bond subscription to the investor/custodian privacy group
  logger.log("Creating private bond subscription...");
  const bondSubscription = await newBondSubscription(
    investorCustodianGroup,
    investor,
    {
      bondAddress_: notoBond.address,
      units_: 100,
      custodian_: await bondCustodian.address(),
      atomFactory_: atomFactoryAddress,
    }
  );
  if (!checkDeploy(bondSubscription)) return false;

  // Prepare the payment transfer (investor -> custodian)
  logger.log("Locking cash transfer from investor...");
  receipt = await notoCash
    .using(paladin3)
    .lock(investor, {
      amount: 100,
      data: "0x",
    })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;
  receipt = await paladin3.ptx.getTransactionReceiptFull(receipt.id);
  let domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const cashLockId = domainReceipt?.lockInfo?.lockId;
  if (cashLockId === undefined) {
    logger.error("No lock ID found in domain receipt");
    return false;
  }
  balanceInvestor = await notoCash
    .using(paladin3)
    .balanceOf(investor, { account: investor.lookup });
  logger.log(
    `(NotoCash) Investor State: ${balanceInvestor.totalBalance} units of cash, ${balanceInvestor.totalStates} states, overflow: ${balanceInvestor.overflow}`
  );

  // Prepare unlock operation
  logger.log("Preparing unlock to bond custodian...");
  receipt = await notoCash
    .using(paladin3)
    .prepareUnlock(investor, {
      lockId: cashLockId,
      from: investor,
      recipients: [{ to: bondCustodian, amount: 100 }],
      data: "0x",
    })
    .waitForReceipt(5000, true);
  if (!checkReceipt(receipt)) return false;
  domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const cashUnlockParams = domainReceipt?.lockInfo?.unlockParams;
  const cashUnlockCall = domainReceipt?.lockInfo?.unlockCall;
  if (cashUnlockParams === undefined || cashUnlockCall === undefined) {
    logger.error("No unlock data found in domain receipt");
    return false;
  }

  // Prepare the bond transfer (custodian -> investor)
  logger.log("Locking bond asset from custodian...");
  receipt = await notoBond
    .using(paladin2)
    .lock(bondCustodian, {
      amount: 100,
      data: "0x",
    })
    .waitForReceipt(5000, true);
  if (!checkReceipt(receipt)) return false;
  domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const bondLockId = domainReceipt?.lockInfo?.lockId;
  if (bondLockId === undefined) {
    logger.error("No lock ID found in domain receipt");
    return false;
  }
  balanceCustodian = await notoBond
    .using(paladin2)
    .balanceOf(bondCustodian, { account: bondCustodian.lookup });
  logger.log(
    `(NotoBond) Bond Custodian State: ${balanceCustodian.totalBalance} units of bonds, ${balanceCustodian.totalStates} states, overflow: ${balanceCustodian.overflow}`
  );

  // Prepare unlock operation
  logger.log("Preparing unlock to investor...");
  receipt = await notoBond
    .using(paladin2)
    .prepareUnlock(bondCustodian, {
      lockId: bondLockId,
      from: bondCustodian,
      recipients: [{ to: investor, amount: 100 }],
      data: "0x",
    })
    .waitForReceipt(5000, true);
  if (!checkReceipt(receipt)) return false;
  domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const assetUnlockParams = domainReceipt?.lockInfo?.unlockParams;
  const assetUnlockCall = domainReceipt?.lockInfo?.unlockCall;
  if (assetUnlockParams === undefined || assetUnlockCall === undefined) {
    logger.error("No unlock data found in domain receipt");
    return false;
  }

  // Pass the prepared payment transfer to the subscription contract
  logger.log("Adding payment information to subscription request...");
  receipt = await bondSubscription
    .using(paladin3)
    .preparePayment(investor, {
      to: notoCash.address,
      encodedCall: cashUnlockCall,
    })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;

  // Pass the prepared bond transfer to the subscription contract
  logger.log("Adding bond information to subscription request...");
  receipt = await bondSubscription
    .using(paladin2)
    .prepareBond(bondCustodian, {
      to: notoBond.address,
      encodedCall: assetUnlockCall,
    })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;

  // Prepare bond distribution (initializes atomic swap of payment and bond units)
  logger.log("Generating atom for bond distribution...");
  receipt = await bondSubscription
    .using(paladin2)
    .distribute(bondCustodian)
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;

  // Extract the address of the created Atom
  const events = await paladin2.bidx.decodeTransactionEvents(
    receipt.transactionHash,
    atomFactoryJson.abi,
    ""
  );
  const atomDeployedEvent = events.find(
    (e) => e.soliditySignature === "event AtomDeployed(address addr)"
  );
  if (atomDeployedEvent === undefined) {
    logger.error("Did not find AtomDeployed event");
    return false;
  }
  const atomAddress = atomDeployedEvent.data.addr;
  logger.log("Success!");

  // Approve the payment transfer
  logger.log("Approving payment transfer...");
  receipt = await notoCash
    .using(paladin3)
    .delegateLock(investor, {
      lockId: cashLockId,
      unlock: cashUnlockParams,
      delegate: atomAddress,
      data: "0x",
    })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;

  // Approve the bond transfer
  logger.log("Approving bond transfer...");
  receipt = await notoBond
    .using(paladin2)
    .delegateLock(bondCustodian, {
      lockId: bondLockId,
      unlock: assetUnlockParams,
      delegate: atomAddress,
      data: "0x",
    })
    .waitForReceipt(10000);
  if (!checkReceipt(receipt)) return false;

  // Execute the atomic transfer
  logger.log("Distributing bond...");
  txID = await paladin2.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: atomJson.abi,
    function: "execute",
    from: bondCustodian.lookup,
    to: atomAddress,
    data: {},
  });
  receipt = await paladin2.pollForReceipt(txID, 10000);
  if (!checkReceipt(receipt)) return false;


  // it can take some time for the balances to update, so loop until all balances are >0
  let finalCashBalanceInvestor: NotoBalanceOfResult | undefined;
  let finalBondBalanceInvestor: NotoBalanceOfResult | undefined;
  let finalCashBalanceCustodian: NotoBalanceOfResult | undefined;
  let finalBondBalanceCustodian: NotoBalanceOfResult | undefined;
  const startTime = Date.now();
  while (true) {
  // Get final balances after the bond distribution
  finalCashBalanceInvestor = await notoCash
    .using(paladin3)
    .balanceOf(investor, { account: investor.lookup });

  finalBondBalanceInvestor = await notoBond
    .using(paladin3)
    .balanceOf(investor, { account: investor.lookup });

  finalCashBalanceCustodian = await notoCash
    .using(paladin2)
    .balanceOf(bondCustodian, { account: bondCustodian.lookup });

    finalBondBalanceCustodian = await notoBond
    .using(paladin2)
    .balanceOf(bondCustodian, { account: bondCustodian.lookup });

    if (finalCashBalanceInvestor?.totalBalance !== "0" &&
      finalBondBalanceInvestor?.totalBalance !== "0" &&
      finalCashBalanceCustodian?.totalBalance !== "0" &&
      finalBondBalanceCustodian?.totalBalance !== "0") {
      break;
    }
    await new Promise(resolve => setTimeout(resolve, 1000));

    if (Date.now() - startTime > 60000) {
      logger.error("Failed to get final balances after 60 seconds");
      return false;
    }
  }

      // Save contract data to file for later use
  const contractData: ContractData = {
    notoCashAddress: notoCash.address,
    notoBondAddress: notoBond.address,
    issuerCustodianGroupId: issuerCustodianGroup.group.id,
    issuerCustodianGroupAddress: issuerCustodianGroup.address,
    investorCustodianGroupId: investorCustodianGroup.group.id,
    investorCustodianGroupAddress: investorCustodianGroup.address,
    bondTrackerAddress: bondTracker.address,
    bondTrackerPublicAddress: bondTrackerPublicAddress,
    bondSubscriptionAddress: bondSubscription.address,
    atomFactoryAddress: atomFactoryAddress,
    atomAddress: atomAddress,
    bondDetails: {
      issueDate: issueDate,
      maturityDate: maturityDate,
      faceValue: 1,
      discountPrice: 1,
      minimumDenomination: 1,
      bondUnits: 100,
      cashAmount: 100
    },
    lockDetails: {
      cashLockId: cashLockId,
      bondLockId: bondLockId,
      cashUnlockCall: cashUnlockCall,
      assetUnlockCall: assetUnlockCall
    },
    finalBalances: {
      cash: {
        investor: {
          totalBalance: finalCashBalanceInvestor.totalBalance,
          totalStates: finalCashBalanceInvestor.totalStates,
          overflow: finalCashBalanceInvestor.overflow
        },
        custodian: {
          totalBalance: finalCashBalanceCustodian.totalBalance,
          totalStates: finalCashBalanceCustodian.totalStates,
          overflow: finalCashBalanceCustodian.overflow
        }
      },
      bond: {
        investor: {
          totalBalance: finalBondBalanceInvestor.totalBalance,
          totalStates: finalBondBalanceInvestor.totalStates,
          overflow: finalBondBalanceInvestor.overflow
        },
        custodian: {
          totalBalance: finalBondBalanceCustodian.totalBalance,
          totalStates: finalBondBalanceCustodian.totalStates,
          overflow: finalBondBalanceCustodian.overflow
        }
      }
    },
    participants: {
      cashIssuer: cashIssuer.lookup,
      bondIssuer: bondIssuer.lookup,
      bondCustodian: bondCustodian.lookup,
      investor: investor.lookup
    },
    timestamp: new Date().toISOString()
  };

  const dataDir = path.join(__dirname, '..', 'data');
  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }

  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const dataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(dataFile, JSON.stringify(contractData, null, 2));
  logger.log(`Contract data saved to ${dataFile}`);

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
