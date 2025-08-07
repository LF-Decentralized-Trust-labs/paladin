import PaladinClient, {
  NotoFactory,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import * as fs from 'fs';
import * as path from 'path';
import { ContractData } from "./tests/data-persistence";

const logger = console;

// Initialize Paladin clients for three nodes
const paladinClientNode1 = new PaladinClient({ url: "http://127.0.0.1:31548" });
const paladinClientNode2 = new PaladinClient({ url: "http://127.0.0.1:31648" });
const paladinClientNode3 = new PaladinClient({ url: "http://127.0.0.1:31748" });

async function main(): Promise<boolean> {
  // Retrieve verifiers for each node
  const [verifierNode1] = paladinClientNode1.getVerifiers("user@node1");
  const [verifierNode2] = paladinClientNode2.getVerifiers("user@node2");
  const [verifierNode3] = paladinClientNode3.getVerifiers("user@node3");

  const mintAmount = 2000;
  const transferToNode2Amount = 1000;
  const transferToNode3Amount = 800;

  // Step 1: Deploy a Noto token to represent cash
  logger.log("Step 1: Deploying a Noto cash token...");
  const notoFactory = new NotoFactory(paladinClientNode1, "noto");
  const cashToken = await notoFactory
    .newNoto(verifierNode1, {
      notary: verifierNode1,
      notaryMode: "basic",
    })
    .waitForDeploy();
  if (!cashToken) {
    logger.error("Failed to deploy the Noto cash token!");
    return false;
  }
  logger.log("Noto cash token deployed successfully!");

 

  // Step 2: Mint cash tokens
  logger.log(`Step 2: Minting ${mintAmount} units of cash to Node1...`);
  const mintReceipt = await cashToken
    .mint(verifierNode1, {
      to: verifierNode1,
      amount: mintAmount,
      data: "0x",
    })
    .waitForReceipt(10000);
  if (!mintReceipt) {
    logger.error("Failed to mint cash tokens!");
    return false;
  }
  logger.log(`Successfully minted ${mintAmount} units of cash to Node1!`);
  let balanceNode1 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode1.lookup,
  });
  logger.log(
    `Node1 State: ${balanceNode1.totalBalance} units of cash, ${balanceNode1.totalStates} states, overflow: ${balanceNode1.overflow}`
  );

  // Step 3: Transfer cash to Node2
  logger.log("Step 3: Transferring 1000 units of cash from Node1 to Node2...");
  const transferToNode2 = await cashToken
    .transfer(verifierNode1, {
      to: verifierNode2,
      amount: transferToNode2Amount,
      data: "0x",
    })
    .waitForReceipt(10000);
  if (!transferToNode2) {
    logger.error("Failed to transfer cash to Node2!");
    return false;
  }
  logger.log(`Successfully transferred ${transferToNode2Amount} units of cash to Node2!`);
  let balanceNode2 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode2.lookup,
  });
  logger.log(
    `Node2 State: ${balanceNode2.totalBalance} units of cash, ${balanceNode2.totalStates} states, overflow: ${balanceNode2.overflow}`
  );

  // Step 4: Transfer cash to Node3 from Node2
  logger.log("Step 4: Transferring 800 units of cash from Node2 to Node3...");
  const transferToNode3 = await cashToken
    .using(paladinClientNode2)
    .transfer(verifierNode2, {
      to: verifierNode3,
      amount: transferToNode3Amount,
      data: "0x",
    })
    .waitForReceipt(10000);
  if (!transferToNode3) {
    logger.error("Failed to transfer cash to Node3!");
    return false;
  }
  logger.log(`Successfully transferred ${transferToNode3Amount} units of cash to Node3!`);
  let balanceNode3 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode3.lookup,
  });
  logger.log(
    `Node3 State: ${balanceNode3.totalBalance} units of cash, ${balanceNode3.totalStates} states, overflow: ${balanceNode3.overflow}`
  );


  const finalBalanceNode1 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode1.lookup,
  });
  const finalBalanceNode2 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode2.lookup,
  });
  const finalBalanceNode3 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode3.lookup,
  });

  // Save contract data to file for later use
  // should be of type
  const contractData : ContractData =  { 
    tokenAddress: cashToken.address,
    notary: verifierNode1.lookup,
    notaryMode: "basic",
    mintAmount: mintAmount,
    transferToNode2Amount: transferToNode2Amount,
    transferToNode3Amount: transferToNode3Amount,
    mintTransactionHash: mintReceipt?.transactionHash,
    transferToNode2TransactionHash: transferToNode2?.transactionHash,
    transferToNode3TransactionHash: transferToNode3?.transactionHash,
    finalBalances: {
      node1: {
        totalBalance: finalBalanceNode1.totalBalance,
        totalStates: finalBalanceNode1.totalStates,
        overflow: finalBalanceNode1.overflow
      },
      node2: {
        totalBalance: finalBalanceNode2.totalBalance,
        totalStates: finalBalanceNode2.totalStates,
        overflow: finalBalanceNode2.overflow
      },
      node3: {
        totalBalance: finalBalanceNode3.totalBalance,
        totalStates: finalBalanceNode3.totalStates,
        overflow: finalBalanceNode3.overflow
      }
    },
    node1Verifier: verifierNode1.lookup,
    node2Verifier: verifierNode2.lookup,
    node3Verifier: verifierNode3.lookup,
    timestamp: new Date().toISOString()
  };

  // Use command-line argument for data directory if provided, otherwise use default
  const dataDir = process.argv[2] || path.join(__dirname, '..', 'data');
  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }

  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const dataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(dataFile, JSON.stringify(contractData, null, 2));
  logger.log(`Contract data saved to ${dataFile}`);

  // All steps completed successfully
  logger.log("All operations completed successfully!");
  return true;
}

// Execute the main function if this file is run directly
if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1); // Exit with 0 for success, 1 for failure
    })
    .catch((err) => {
      logger.error("Exiting due to an uncaught error:", err);
      process.exit(1); // Exit with status 1 for any uncaught errors
    });
}
