import PaladinClient, {
  encodeStates,
  newGroupSalt,
  newTransactionId,
  NotoFactory,
  PenteFactory,
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import bondTrackerPublicJson from "./abis/BondTrackerPublic.json";
import atomFactoryJson from "./abis/AtomFactory.json";
import atomJson from "./abis/Atom.json";
import { newBondSubscription } from "./helpers/bondsubscription";
import { newBondTracker } from "./helpers/bondtracker";
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
  const [cashIssuer, bondIssuer] = paladin1.getVerifiers(
    "cashIssuer@node1",
    "bondIssuer@node1"
  );
  const [bondCustodian] = paladin2.getVerifiers("bondCustodian@node2");
  const [investor] = paladin3.getVerifiers("investor@node3");

  // Create a Noto token to represent cash
  logger.log("Deploying Noto cash token...");
  const notoFactory = new NotoFactory(paladin1, "noto");
  const notoCash = await notoFactory.newNoto(cashIssuer, {
    notary: cashIssuer,
    restrictMinting: true,
  });
  if (!checkDeploy(notoCash)) return false;

  // Issue some cash
  logger.log("Issuing cash...");
  let receipt = await notoCash.mint(cashIssuer, {
    to: investor,
    amount: 100000,
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  // Create a Pente privacy group between the bond issuer and bond custodian
  logger.log("Creating issuer+custodian privacy group...");
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerCustodianGroup = await penteFactory.newPrivacyGroup(bondIssuer, {
    group: {
      salt: newGroupSalt(),
      members: [bondIssuer, bondCustodian],
    },
    evmVersion: "shanghai",
    endorsementType: "group_scoped_identities",
    externalCallsEnabled: true,
  });
  if (!checkDeploy(issuerCustodianGroup)) return false;

  // Deploy the public bond tracker on the base ledger (controlled by the privacy group)
  logger.log("Creating public bond tracker...");
  const issueDate = Math.floor(Date.now() / 1000);
  const maturityDate = issueDate + 60 * 60 * 24;
  let txID = await paladin1.sendTransaction({
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
  const notoBond = await notoFactory.newNoto(bondIssuer, {
    notary: bondCustodian,
    hooks: {
      privateGroup: issuerCustodianGroup.group,
      publicAddress: issuerCustodianGroup.address,
      privateAddress: bondTracker.address,
    },
    restrictMinting: false,
  });
  if (!checkDeploy(notoBond)) return false;

  // Deploy the atom factory on the base ledger
  logger.log("Creating atom factory...");
  txID = await paladin1.sendTransaction({
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
  receipt = await notoBond.mint(bondIssuer, {
    to: bondCustodian,
    amount: 1000,
    data: "0x",
  });
  if (!checkReceipt(receipt)) return false;

  // Begin bond distribution to investors
  logger.log("Beginning distribution...");
  receipt = await bondTracker.using(paladin2).beginDistribution(bondCustodian, {
    discountPrice: 1,
    minimumDenomination: 1,
  });
  if (!checkReceipt(receipt)) return false;

  // Add allowed investors
  const investorList = await bondTracker.investorList(bondIssuer);
  await investorList
    .using(paladin2)
    .addInvestor(bondCustodian, { addr: await investor.address() });

  // Create a Pente privacy group between the bond investor and bond custodian
  logger.log("Creating investor+custodian privacy group...");
  const investorCustodianGroup = await penteFactory
    .using(paladin3)
    .newPrivacyGroup(investor, {
      group: {
        salt: newGroupSalt(),
        members: [investor, bondCustodian],
      },
      evmVersion: "shanghai",
      endorsementType: "group_scoped_identities",
      externalCallsEnabled: true,
    });
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
  logger.log("Preparing payment transfer...");
  const paymentTransfer = await notoCash
    .using(paladin3)
    .prepareTransfer(investor, {
      to: bondCustodian,
      amount: 100,
      data: "0x",
    });
  if (paymentTransfer === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log("Success!");

  if (paymentTransfer.transaction.to === undefined) {
    logger.error("Prepared payment transfer had no 'to' address");
    return false;
  }

  // Prepare the bond transfer (custodian -> investor)
  // Requires 2 calls to prepare, as the Noto transaction spawns a Pente transaction to wrap it
  logger.log("Preparing bond transfer (step 1/2)...");
  const bondTransfer1 = await notoBond
    .using(paladin2)
    .prepareTransfer(bondCustodian, {
      to: investor,
      amount: 100,
      data: "0x",
    });
  if (bondTransfer1 === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log("Success!");

  if (bondTransfer1.transaction.type !== TransactionType.PRIVATE) {
    logger.error(
      `Prepared bond transfer did not result in a private Pente transaction: ${bondTransfer1.transaction}`
    );
    return false;
  }

  logger.log("Preparing bond transfer (step 2/2)...");
  txID = await paladin2.prepareTransaction(bondTransfer1.transaction);
  const bondTransfer2 = await paladin2.pollForPreparedTransaction(txID, 10000);
  if (bondTransfer2 === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log("Success!");

  if (bondTransfer2.transaction.to === undefined) {
    logger.error("Prepared bond transfer had no 'to' address");
    return false;
  }
  if (!bondTransfer2.transaction.function.startsWith("transition(")) {
    logger.error(
      `Prepared bond transfer did not seem to be a Pente transition: ${bondTransfer2.transaction}`
    );
    return false;
  }

  // Pass the prepared payment transfer to the subscription contract
  logger.log("Adding payment information to subscription request...");
  receipt = await bondSubscription.using(paladin3).preparePayment(investor, {
    to: paymentTransfer.transaction.to,
    encodedCall: paymentTransfer.metadata?.transferWithApproval?.encodedCall,
  });
  if (!checkReceipt(receipt)) return false;

  // Pass the prepared bond transfer to the subscription contract
  logger.log("Adding bond information to subscription request...");
  receipt = await bondSubscription.using(paladin2).prepareBond(bondCustodian, {
    to: bondTransfer2.transaction.to,
    encodedCall: bondTransfer2.metadata.transitionWithApproval.encodedCall,
  });
  if (!checkReceipt(receipt)) return false;

  // Prepare bond distribution (initializes atomic swap of payment and bond units)
  logger.log("Generating atom for bond distribution...");
  receipt = await bondSubscription.using(paladin2).distribute(bondCustodian);
  if (!checkReceipt(receipt)) return false;

  // Extract the address of the created Atom
  const events = await paladin2.decodeTransactionEvents(
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
  receipt = await notoCash.using(paladin3).approveTransfer(investor, {
    inputs: encodeStates(paymentTransfer.states.spent ?? []),
    outputs: encodeStates(paymentTransfer.states.confirmed ?? []),
    data: paymentTransfer.metadata.approvalParams.data,
    delegate: atomAddress,
  });
  if (!checkReceipt(receipt)) return false;

  // Approve the bond transfer
  logger.log("Approving bond transfer...");
  receipt = await issuerCustodianGroup
    .using(paladin2)
    .approveTransition(bondCustodian, {
      txId: newTransactionId(),
      transitionHash: bondTransfer2.metadata.approvalParams.transitionHash,
      signatures: bondTransfer2.metadata.approvalParams.signatures,
      delegate: atomAddress,
    });
  if (!checkReceipt(receipt)) return false;

  // Execute the atomic transfer
  logger.log("Distributing bond...");
  txID = await paladin2.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: atomJson.abi,
    function: "execute",
    from: bondCustodian.lookup,
    to: atomAddress,
    data: {},
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
