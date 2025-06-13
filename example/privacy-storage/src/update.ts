import PaladinClient, {
  PenteFactory,
  PentePrivacyGroup,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { checkDeploy } from "paladin-example-common";
import storageJson from "./abis/Storage.json";
import { PrivateStorage } from "./helpers/storage";

const logger = console;

// Initialize Paladin clients for three nodes
const paladinNode1 = new PaladinClient({ url: "http://127.0.0.1:31548" });
const paladinNode2 = new PaladinClient({ url: "http://127.0.0.1:31648" });
const paladinNode3 = new PaladinClient({ url: "http://127.0.0.1:31748" });

async function main(): Promise<boolean> {
  // Get the first argument from the command line
  if (process.argv.length !== 4) {
    logger.error(
      "To run the update example, pass the privacy group ID and private contract address from a previous invocation of `npm run start`:"
    );
    logger.error(
      "E.g: node run update <privacy group ID> <private contract address>"
    );
    return false;
  }
  const groupId = process.argv[2];
  const contractAddress = process.argv[3];

  // Get verifiers for each node
  const [verifierNode1] = paladinNode1.getVerifiers("member@node1");
  const [verifierNode2] = paladinNode2.getVerifiers("member@node2");
  const [verifierNode3] = paladinNode3.getVerifiers("outsider@node3");

  // Step 1: Create a privacy group for members
  logger.log(`Resuming privacy group ${groupId} for Node1 and Node2...`);
  const penteFactory = new PenteFactory(paladinNode1, "pente");
  const existingPrivacyMemberGroup = await penteFactory.resumePrivacyGroup({
    id: groupId,
  });

  if (existingPrivacyMemberGroup === undefined) {
    logger.error(`Failed to resume privacy group ${groupId}`);
    return false;
  }

  // Step 3: Use the deployed contract for private storage
  const privateStorageContract = new PrivateStorage(
    existingPrivacyMemberGroup,
    contractAddress
  );

  logger.log(
    `Using existing private member group ${groupId}, with existing storage contract address ${privateStorageContract.address}`
  );

  // Retrieve the current value as Node1
  logger.log("Node1 retrieving the current value from the contract...");
  let retrievedValueNode1 = await privateStorageContract.call({
    from: verifierNode1.lookup,
    function: "retrieve",
  });
  logger.log(
    "Node1 retrieved the value successfully:",
    retrievedValueNode1["value"]
  );

  // Store a new value in the contract
  const valueToAdd = Math.floor(Math.random() * 100);
  logger.log(
    `Adding ${valueToAdd} to the current value and storing the new value`
  );
  const valueToStore = parseInt(retrievedValueNode1["value"]) + valueToAdd;

  logger.log(`Storing the new value "${valueToStore}" in the contract...`);
  const storeTx = await privateStorageContract.sendTransaction({
    from: verifierNode1.lookup,
    function: "store",
    data: { num: valueToStore },
  });
  logger.log(
    "Value stored successfully! Transaction hash:",
    storeTx?.transactionHash
  );

  // Retrieve the value as Node1
  logger.log("Node1 retrieving the new value from the contract...");
  retrievedValueNode1 = await privateStorageContract.call({
    from: verifierNode1.lookup,
    function: "retrieve",
  });
  logger.log(
    "Node1 retrieved the value successfully:",
    retrievedValueNode1["value"]
  );

  // Retrieve the value as Node2
  logger.log("Node2 retrieving the value from the contract...");
  const retrievedValueNode2 = await privateStorageContract
    .using(paladinNode2)
    .call({
      from: verifierNode2.lookup,
      function: "retrieve",
    });
  logger.log(
    "Node2 retrieved the value successfully:",
    retrievedValueNode2["value"]
  );

  // Attempt to retrieve the value as Node3 (outsider)
  try {
    logger.log("Node3 (outsider) attempting to retrieve the value...");
    await privateStorageContract.using(paladinNode3).call({
      from: verifierNode3.lookup,
      function: "retrieve",
    });
    logger.error(
      "Node3 (outsider) should not have access to the privacy group!"
    );
    return false;
  } catch (error) {
    logger.info(
      "Expected behavior - Node3 (outsider) cannot retrieve the data from the privacy group. Access denied."
    );
  }

  logger.log("All steps completed successfully!");

  return true;
}

// Execute the main function when this file is run directly
if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1); // Exit with status 0 for success, 1 for failure
    })
    .catch((err) => {
      logger.error("Exiting due to an uncaught error:", err);
      process.exit(1); // Exit with status 1 for any uncaught errors
    });
}
