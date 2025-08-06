import PaladinClient, {
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import storageJson from "./abis/Storage.json";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections } from "../../common/src/config";

const logger = console;

// ASCII art for EVM
const evmArt = `
███████╗  ██╗   ██╗  ███╗   ███╗
██╔════╝  ██║   ██║  ████╗ ████║
█████╗    ██║   ██║  ██╔████╔██║
██╔══╝    ╚██╗ ██╔╝  ██║╚██╔╝██║
███████╗   ╚████╔╝   ██║ ╚═╝ ██║
╚══════╝    ╚═══╝    ╚═╝     ╚═╝
`;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  logger.log(evmArt);
  logger.log("==================================================================================");
  logger.log("🛡️  Initializing Paladin Client for Public Storage Demonstration 🛡️");
  logger.log("==================================================================================");

  if (nodeConnections.length < 1) {
    logger.error("The environment config must provide at least 1 node for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin client from the environment configuration...");
  const paladin = new PaladinClient(nodeConnections[0].clientOptions);
  const [owner] = paladin.getVerifiers(`owner@${nodeConnections[0].id}`);

  // Step 1: Deploy the Storage contract
  logger.log("\n==================================================================================");
  logger.log("📜 Step 1: Deploying the Public Storage contract...");
  logger.log("==================================================================================");
  const deploymentTxID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: storageJson.abi,
    bytecode: storageJson.bytecode,
    from: owner.lookup,
    data: {},
  });

  // Wait for deployment receipt
  const deploymentReceipt = await paladin.pollForReceipt(deploymentTxID, 10000);
  if (!deploymentReceipt?.contractAddress) {
    logger.error("Deployment failed!");
    return false;
  }
  logger.log(`✅ Public Storage contract deployed successfully! Address: ${deploymentReceipt.contractAddress}`);

  // Step 2: Store a value in the contract
  const valueToStore = 125; // Example value to store
  logger.log("\n==================================================================================");
  logger.log(`✍️  Step 2: Storing value "${valueToStore}" in the contract...`);
  logger.log("==================================================================================");
  const storeTxID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: storageJson.abi,
    function: "store",
    from: owner.lookup,
    to: deploymentReceipt.contractAddress,
    data: { num: valueToStore },
  });

  // Wait for the store transaction receipt
  const storeReceipt = await paladin.pollForReceipt(storeTxID, 10000);
  if (!storeReceipt?.transactionHash) {
    logger.error("Failed to store value in the contract!");
    return false;
  }
  logger.log(`✅ Value stored successfully! Transaction Hash: ${storeReceipt.transactionHash}`);

  // Step 3: Retrieve the stored value from the contract
  logger.log("\n==================================================================================");
  logger.log("🧐 Step 3: Retrieving the stored value...");
  logger.log("==================================================================================");
  const retrieveResult = await paladin.ptx.call({
    type: TransactionType.PUBLIC,
    abi: storageJson.abi,
    function: "retrieve",
    from: owner.lookup,
    to: deploymentReceipt.contractAddress,
    data: {},
  });

  // Validate the retrieved value
  const retrievedValue = retrieveResult["value"];
  if (retrievedValue !== valueToStore.toString()) {
    logger.error(`Retrieved value "${retrievedValue}" does not match stored value "${valueToStore}"!`);
    return false;
  }

  logger.log(`✅ Value retrieved successfully! Retrieved value: "${retrievedValue}"`);

  // Save contract data to file for later use
  const contractData = {
    contractAddress: deploymentReceipt.contractAddress,
    storedValue: valueToStore,
    retrievedValue: retrievedValue,
    storeTransactionHash: storeReceipt.transactionHash,
    timestamp: new Date().toISOString()
  };

  const dataDir = path.join(__dirname, '..', 'data');
  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }

  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const dataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(dataFile, JSON.stringify(contractData, null, 2));
  logger.log(`\n💾 Contract data saved to ${dataFile}`);

  logger.log("\n==================================================================================");
  logger.log("🎉 All steps completed successfully!");
  logger.log("==================================================================================");

  return true;
}

// Entry point
if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      logger.error("Exiting with uncaught error");
      logger.error(err);
      process.exit(1);
    });
}
