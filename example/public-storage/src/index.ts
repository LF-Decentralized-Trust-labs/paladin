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
  logger.log("Step 1: Storage contract deployed successfully!");

  // Step 3: Store a value in the contract
  const valueToStore = 125; // Example value to store
  logger.log(`Step 2: Storing value "${valueToStore}" in the contract...`);
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
  logger.log("Step 2: Value stored successfully!" );

  // Step 4: Retrieve the stored value from the contract
  logger.log("Step 3: Retrieving the stored value...");
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
