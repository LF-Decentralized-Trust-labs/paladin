import PaladinClient from "@lfdecentralizedtrust-labs/paladin-sdk";
import storageJson from "./abis/Storage.json";
import * as fs from 'fs';
import * as path from 'path';

const logger = console;

// Instantiate Paladin client
const paladin = new PaladinClient({
  url: "http://127.0.0.1:31548",
});

interface ContractData {
  contractAddress: string;
  storedValue: number;
  retrievedValue: string;
  storeTransactionHash: string;
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
  logger.log(`Contract Address: ${contractData.contractAddress}`);
  logger.log(`Stored Value: ${contractData.storedValue}`);
  logger.log(`Retrieved Value: ${contractData.retrievedValue}`);
  logger.log(`Store Transaction Hash: ${contractData.storeTransactionHash}`);

  // STEP 2: Verify the stored value is still accessible
  logger.log("STEP 2: Verifying the stored value is still accessible...");
  const [owner] = paladin.getVerifiers("owner@node1");
  
  try {
    const retrieveResult = await paladin.ptx.call({
      type: "public" as any,
      abi: storageJson.abi,
      function: "retrieve",
      from: owner.lookup,
      to: contractData.contractAddress,
      data: {},
    });

    const currentValue = retrieveResult["value"];
    logger.log(`STEP 2: Current stored value: "${currentValue}"`);

    if (currentValue !== contractData.retrievedValue) {
      logger.error(`STEP 2: ERROR - Current value does not match saved value!`);
      logger.error(`Expected: "${contractData.retrievedValue}"`);
      logger.error(`Found: "${currentValue}"`);
      return false;
    }

    logger.log("STEP 2: Stored value verification successful!");

  } catch (error) {
    logger.error("STEP 2: Failed to retrieve stored value!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 3: Query transaction receipts for the store transaction
  logger.log("STEP 3: Querying transaction receipts...");
  try {
    const receipts = await paladin.ptx.queryTransactionReceipts({
      limit: 10,
      sort: ["sequence"],
      equal: [
        {
          field: "transactionHash",
          value: contractData.storeTransactionHash
        }
      ]
    });

    if (receipts.length === 0) {
      logger.error("STEP 3: No transaction receipts found");
      return false;
    }
    
    logger.log(`STEP 3: Found ${receipts.length} transaction receipts`);
    const receipt = receipts[0];
    logger.log(`Transaction ID: ${receipt.id}`);
    logger.log(`Success: ${receipt.success}`);
    logger.log(`Block Number: ${receipt.blockNumber}`);

  } catch (error) {
    logger.error("STEP 3: Failed to query transaction receipts!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 4: Test contract functionality by storing a new value
  logger.log("STEP 4: Testing contract functionality with a new value...");
  try {
    const newValueToStore = 999;
    logger.log(`STEP 4: Storing new value "${newValueToStore}" in the contract...`);
    
    const storeTxID = await paladin.ptx.sendTransaction({
      type: "public" as any,
      abi: storageJson.abi,
      function: "store",
      from: owner.lookup,
      to: contractData.contractAddress,
      data: { num: newValueToStore },
    });

    if (!storeTxID) {
      logger.error("STEP 4: Function call failed!");
      return false;
    }

    // Wait for the store transaction receipt
    const storeReceipt = await paladin.pollForReceipt(storeTxID, 10000);
    if (!storeReceipt?.transactionHash) {
      logger.error("STEP 4: Receipt retrieval failed!");
      return false;
    }
    logger.log("STEP 4: New value stored successfully!");

    // Verify the new value was stored correctly
    logger.log("STEP 4: Verifying the new value was stored correctly...");
    const newRetrieveResult = await paladin.ptx.call({
      type: "public" as any,
      abi: storageJson.abi,
      function: "retrieve",
      from: owner.lookup,
      to: contractData.contractAddress,
      data: {},
    });

    const newRetrievedValue = newRetrieveResult["value"];
    if (newRetrievedValue !== newValueToStore.toString()) {
      logger.error(`STEP 4: ERROR - New value verification failed!`);
      logger.error(`Expected: "${newValueToStore}"`);
      logger.error(`Found: "${newRetrievedValue}"`);
      return false;
    }

    logger.log("STEP 4: New value verification successful!");
    logger.log(`New stored value: "${newRetrievedValue}"`);
    logger.log(`New transaction hash: ${storeReceipt.transactionHash}`);

  } catch (error) {
    logger.error("STEP 4: Contract functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 5: Restore the original value
  logger.log("STEP 5: Restoring the original value...");
  try {
    logger.log(`STEP 5: Restoring original value "${contractData.storedValue}"...`);
    
    const restoreTxID = await paladin.ptx.sendTransaction({
      type: "public" as any,
      abi: storageJson.abi,
      function: "store",
      from: owner.lookup,
      to: contractData.contractAddress,
      data: { num: contractData.storedValue },
    });

    if (!restoreTxID) {
      logger.error("STEP 5: Function call failed!");
      return false;
    }

    // Wait for the restore transaction receipt
    const restoreReceipt = await paladin.pollForReceipt(restoreTxID, 10000);
    if (!restoreReceipt?.transactionHash) {
      logger.error("STEP 5: Receipt retrieval failed!");
      return false;
    }
    logger.log("STEP 5: Original value restored successfully!");

    // Final verification
    const finalRetrieveResult = await paladin.ptx.call({
      type: "public" as any,
      abi: storageJson.abi,
      function: "retrieve",
      from: owner.lookup,
      to: contractData.contractAddress,
      data: {},
    });

    const finalValue = finalRetrieveResult["value"];
    if (finalValue !== contractData.retrievedValue) {
      logger.error(`STEP 5: ERROR - Final value verification failed!`);
      logger.error(`Expected: "${contractData.retrievedValue}"`);
      logger.error(`Found: "${finalValue}"`);
      return false;
    }

    logger.log("STEP 5: Original value restoration verification successful!");

  } catch (error) {
    logger.error("STEP 5: Value restoration failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  logger.log("\nSUCCESS: Historical verification completed!");
  logger.log(`Contract address: ${contractData.contractAddress}`);
  logger.log(`Original stored value: ${contractData.storedValue}`);
  logger.log(`Original retrieved value: "${contractData.retrievedValue}"`);
  logger.log(`Store transaction hash: ${contractData.storeTransactionHash}`);
  logger.log(`Deployed at: ${contractData.timestamp}`);

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