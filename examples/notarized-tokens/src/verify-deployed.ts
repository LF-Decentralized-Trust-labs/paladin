import PaladinClient, {
  NotoFactory,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import * as fs from 'fs';
import * as path from 'path';

const logger = console;

// Initialize Paladin clients for three nodes
const paladinClientNode1 = new PaladinClient({ url: "http://127.0.0.1:31548" });
const paladinClientNode2 = new PaladinClient({ url: "http://127.0.0.1:31648" });
const paladinClientNode3 = new PaladinClient({ url: "http://127.0.0.1:31748" });

export interface ContractData {
  tokenAddress: string;
  notary: string;
  notaryMode: string;
  mintAmount: number;
  transferToNode2Amount: number;
  transferToNode3Amount: number;
  mintTransactionHash: string;
  transferToNode2TransactionHash: string;
  transferToNode3TransactionHash: string;
  finalBalances: {
    node1: {
      totalBalance: string;
      totalStates: string;
      overflow: boolean;
    };
    node2: {
      totalBalance: string;
      totalStates: string;
      overflow: boolean;
    };
    node3: {
      totalBalance: string;
      totalStates: string;
      overflow: boolean;
    };
  };
  node1Verifier: string;
  node2Verifier: string;
  node3Verifier: string;
  timestamp: string;
}

function findLatestContractDataFile(dataDir: string): string | null {
  if (!fs.existsSync(dataDir)) {
    return null;
  }

  const files = fs.readdirSync(dataDir)
    .filter(file => file.startsWith('contract-data-') && file.endsWith('.json'))
    .sort()
    .reverse(); // Most recent first

  return files.length > 0 ? path.join(dataDir, files[0]) : null;
}

async function main(): Promise<boolean> {
  // STEP 1: Load the saved contract data
  logger.log("STEP 1: Loading saved contract data...");
  const dataDir = path.join(__dirname, '..', 'data');
  const dataFile = findLatestContractDataFile(dataDir);
  
  if (!dataFile) {
    logger.error(`STEP 1: No contract data files found in ${dataDir}`);
    logger.error("Please run the original script first to deploy the contract and save the data.");
    return false;
  }

  const contractData: ContractData = JSON.parse(fs.readFileSync(dataFile, 'utf8'));
  logger.log(`STEP 1: Loaded contract data from ${dataFile}`);
  logger.log(`Token Address: ${contractData.tokenAddress}`);
  logger.log(`Notary: ${contractData.notary}`);
  logger.log(`Notary Mode: ${contractData.notaryMode}`);
  logger.log(`Mint Amount: ${contractData.mintAmount}`);
  logger.log(`Transfer to Node2 Amount: ${contractData.transferToNode2Amount}`);
  logger.log(`Transfer to Node3 Amount: ${contractData.transferToNode3Amount}`);

  // STEP 2: Get verifiers and recreate token connection
  logger.log("STEP 2: Recreating token connection...");
  const [verifierNode1] = paladinClientNode1.getVerifiers("user@node1");
  const [verifierNode2] = paladinClientNode2.getVerifiers("user@node2");
  const [verifierNode3] = paladinClientNode3.getVerifiers("user@node3");

  // Import NotoInstance from the SDK
  const { NotoInstance } = await import("@lfdecentralizedtrust-labs/paladin-sdk");
  const cashToken = new NotoInstance(paladinClientNode1, contractData.tokenAddress);
  
  if (!cashToken) {
    logger.error("STEP 2: Failed to retrieve token!");
    return false;
  }

  logger.log("STEP 2: Token connection recreated successfully!");

  // STEP 3: Verify current balances match saved data
  logger.log("STEP 3: Verifying current balances...");
  try {
    const currentBalanceNode1 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode1.lookup,
    });

    const currentBalanceNode2 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode2.lookup,
    });

    const currentBalanceNode3 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode3.lookup,
    });

    logger.log(`STEP 3: Current balances:`);
    logger.log(`Node1: ${currentBalanceNode1.totalBalance} units, ${currentBalanceNode1.totalStates} states, overflow: ${currentBalanceNode1.overflow}`);
    logger.log(`Node2: ${currentBalanceNode2.totalBalance} units, ${currentBalanceNode2.totalStates} states, overflow: ${currentBalanceNode2.overflow}`);
    logger.log(`Node3: ${currentBalanceNode3.totalBalance} units, ${currentBalanceNode3.totalStates} states, overflow: ${currentBalanceNode3.overflow}`);

    // Verify balances match saved data
    if (currentBalanceNode1.totalBalance !== contractData.finalBalances.node1.totalBalance) {
      logger.error(`STEP 3: ERROR - Node1 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.node1.totalBalance}`);
      logger.error(`Found: ${currentBalanceNode1.totalBalance}`);
      return false;
    }

    if (currentBalanceNode2.totalBalance !== contractData.finalBalances.node2.totalBalance) {
      logger.error(`STEP 3: ERROR - Node2 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.node2.totalBalance}`);
      logger.error(`Found: ${currentBalanceNode2.totalBalance}`);
      return false;
    }

    if (currentBalanceNode3.totalBalance !== contractData.finalBalances.node3.totalBalance) {
      logger.error(`STEP 3: ERROR - Node3 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.node3.totalBalance}`);
      logger.error(`Found: ${currentBalanceNode3.totalBalance}`);
      return false;
    }

    logger.log("STEP 3: Balance verification successful!");

  } catch (error) {
    logger.error("STEP 3: Failed to retrieve balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 4: Test token functionality by performing a new transfer
  logger.log("STEP 4: Testing token functionality with a new transfer...");
  try {
    const testTransferAmount = 50;
    logger.log(`STEP 4: Transferring ${testTransferAmount} units from Node1 to Node2...`);
    
    const testTransferReceipt = await cashToken
      .transfer(verifierNode1, {
        to: verifierNode2,
        amount: testTransferAmount,
        data: "0x",
      })
      .waitForReceipt(10000);

    if (!testTransferReceipt?.transactionHash) {
      logger.error("STEP 4: Test transfer failed!");
      return false;
    }

    logger.log("STEP 4: Test transfer completed successfully!");

    // Verify the transfer worked by checking new balances
    const newBalanceNode1 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode1.lookup,
    });

    const newBalanceNode2 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode2.lookup,
    });

    const expectedNewBalanceNode1 = Number(contractData.finalBalances.node1.totalBalance) - testTransferAmount;
    const expectedNewBalanceNode2 = Number(contractData.finalBalances.node2.totalBalance) + testTransferAmount;

    if (Number(newBalanceNode1.totalBalance) !== expectedNewBalanceNode1) {
      logger.error(`STEP 4: ERROR - Node1 balance after test transfer is incorrect!`);
      logger.error(`Expected: ${expectedNewBalanceNode1}`);
      logger.error(`Found: ${newBalanceNode1.totalBalance}`);
      return false;
    }

    if (Number(newBalanceNode2.totalBalance) !== expectedNewBalanceNode2) {
      logger.error(`STEP 4: ERROR - Node2 balance after test transfer is incorrect!`);
      logger.error(`Expected: ${expectedNewBalanceNode2}`);
      logger.error(`Found: ${newBalanceNode2.totalBalance}`);
      return false;
    }

    logger.log("STEP 4: Test transfer verification successful!");
    logger.log(`New Node1 balance: ${newBalanceNode1.totalBalance}`);
    logger.log(`New Node2 balance: ${newBalanceNode2.totalBalance}`);
    logger.log(`Test transfer transaction hash: ${testTransferReceipt.transactionHash}`);

  } catch (error) {
    logger.error("STEP 4: Token functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }
 
  logger.log("\nSUCCESS: Verification completed!");

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