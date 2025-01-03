import PaladinClient, {
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import helloWorldJson from "./abis/HelloWorld.json";

const logger = console;

// Instantiate Paladin client (e.g., if you're connecting to "node1")
const paladin = new PaladinClient({
  url: "http://127.0.0.1:31548",
});

async function main(): Promise<boolean> {

  // STEP 1: Send the Deployment Transaction
  let txID = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,      // Deploy publicly
    abi: helloWorldJson.abi,           // ABI of our HelloWorld contract
    bytecode: helloWorldJson.bytecode, // Compiled bytecode
    function: "",                      // No constructor args in this example
    from: "owner",                     // Account that signs and endorses the transaction
    data: {},                          // Additional data if needed (none here)
  });

  // STEP 2: Confirm the Deployment
  // pollForReceipt waits for the transaction to be mined and returns a receipt.
  let receipt = await paladin.pollForReceipt(txID, 10000);
  if (!receipt?.contractAddress) {
    logger.error("Deployment failed!");
    return false;
  }
  logger.log(`Contract deployed successfully! Address: ${receipt.contractAddress}`);

  // STEP 3: Retrieve and Inspect the Transaction
  // getTransaction fetches the transaction details. The second argument (true) 
  // requests extra info. For a minimal response object, use false.
  let result = paladin.getTransaction(txID, true);
  logger.log("Result of getTransaction:", result);

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
