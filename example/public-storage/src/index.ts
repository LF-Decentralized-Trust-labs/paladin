import PaladinClient, {
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import storageJson from "./abis/Storage.json";

const logger = console;

// Instantiate Paladin client
const paladin = new PaladinClient({
  url: "http://127.0.0.1:31548",
});

async function main(): Promise<boolean> {
  // Get the owner account verifier
  const [owner] = paladin.getVerifiers("owner@node1");

  // Step 1: Deploy the Storage contract
  logger.log("Step 1: Deploying the Storage contract...");
  const deploymentTxID = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,       // Public deployment
    abi: storageJson.abi,              // ABI of the Storage contract
    bytecode: storageJson.bytecode,    // Compiled bytecode
    function: "",                      // No constructor arguments
    from: owner.lookup,                // Account signing the transaction
    data: {},                          // No additional data
  });

  // Wait for deployment receipt
  const deploymentReceipt = await paladin.pollForReceipt(deploymentTxID, 10000);
  if (!deploymentReceipt?.contractAddress) {
    logger.error("Deployment failed!");
    return false;
  }
  logger.log("Step 1: Storage contract deployed successfully!");

  // Step 3: Store a value in the contract
  const valueToStore = 125; // Example value to store
  logger.log(`Step 2: Storing value "${valueToStore}" in the contract...`);
  const storeTxID = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,       // Public transaction
    abi: storageJson.abi,               // ABI of the Storage contract
    function: "store",                  // Name of the function to call
    from: owner.lookup,                 // Account signing the transaction
    to: deploymentReceipt.contractAddress, // Address of the deployed contract
    data: { num: valueToStore },        // Function arguments
  });

  // Wait for the store transaction receipt
  const storeReceipt = await paladin.pollForReceipt(storeTxID, 10000);
  if (!storeReceipt?.transactionHash) {
    logger.error("Failed to store value in the contract!");
    return false;
  }
  logger.log("Step 2: Value stored successfully!" );

  // Step 4: Retrieve the stored value from the contract
  logger.log("Step 3: Retrieving the stored value...");
  const retrieveResult = await paladin.call({
    type: TransactionType.PUBLIC,       // Public call
    abi: storageJson.abi,               // ABI of the Storage contract
    function: "retrieve",               // Name of the function to call
    from: owner.lookup,                 // Account making the call
    to: deploymentReceipt.contractAddress, // Address of the deployed contract
    data: {},                           // No arguments required for this function
  });

  // Validate the retrieved value
  const retrievedValue = retrieveResult["0"];
  if (retrievedValue !== valueToStore.toString()) {
    logger.error(`Retrieved value "${retrievedValue}" does not match stored value "${valueToStore}"!`);
    return false;
  }

  logger.log(`Step 3: Value retrieved successfully! Retrieved value: "${retrievedValue}"`);

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
