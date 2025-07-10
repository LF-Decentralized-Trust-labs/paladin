import PaladinClient, {
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import helloWorldJson from "./abis/HelloWorld.json";

const logger = console;

// Instantiate Paladin client (e.g., connecting to "node1")
const paladin = new PaladinClient({
  url: "http://127.0.0.1:31548",
});

async function main(): Promise<boolean> {
  // Retrieve the verifier for the owner account
  const [owner] = paladin.getVerifiers("owner@node1");

  // STEP 1: Deploy the HelloWorld contract
  logger.log("STEP 1: Deploying the HelloWorld contract...");
  const deploymentTxID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: helloWorldJson.abi,
    bytecode: helloWorldJson.bytecode,
    from: owner.lookup,
    data: {},
  });

  // Wait for the deployment receipt
  const deploymentReceipt = await paladin.pollForReceipt(deploymentTxID, 10000, true);
  if (!deploymentReceipt?.contractAddress) {
    logger.error("STEP 1: Deployment failed!");
    return false;
  }
  logger.log("STEP 1: HelloWorld contract deployed successfully!");

  // STEP 2: Call the sayHello function
  logger.log("STEP 2: Calling the sayHello function...");
  const name = "John"; // Example name for the greeting

  const sayHelloTxID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: helloWorldJson.abi,
    function: "sayHello",
    from: owner.lookup,
    to: deploymentReceipt.contractAddress,
    data: {
      name: name,
    },
  });

  if (!sayHelloTxID) {
    logger.error("STEP 2: Function call failed!");
    return false;
  }

  // Wait for the function call receipt
  const functionReceipt = await paladin.pollForReceipt(sayHelloTxID, 10000, true);
  if (!functionReceipt?.transactionHash) {
    logger.error("STEP 2: Receipt retrieval failed!");
    return false;
  }
  logger.log("STEP 2: sayHello function executed successfully!");

  // STEP 3: Retrieve and verify the emitted event
  logger.log("STEP 3: Retrieving and verifying emitted events...");
  const events = await paladin.bidx.decodeTransactionEvents(
    functionReceipt.transactionHash,
    helloWorldJson.abi,
    "pretty=true",
  );

  // Extract the event message and validate its content
  const message = events[0].data["message"];
  const expectedOutput = `Welcome to Paladin, ${name}`;
  if (message !== expectedOutput) {
    logger.error(`STEP 3: ERROR - Event data does not match the expected output! message: "${message}"`);
    return false;
  }
  logger.log("STEP 3: Events verified successfully!");

  // Log the final message to the console
  logger.log("\n", message, "\n");

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
