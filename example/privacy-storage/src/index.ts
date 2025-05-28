import PaladinClient, {
  PenteFactory,
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
  // Get verifiers for each node
  const [verifierNode1] = paladinNode1.getVerifiers("member@node1");
  const [verifierNode2] = paladinNode2.getVerifiers("member@node2");
  const [verifierNode3] = paladinNode3.getVerifiers("outsider@node3");

  // Step 1: Create a privacy group for members
  logger.log("Creating a privacy group for Node1 and Node2...");
  const penteFactory = new PenteFactory(paladinNode1, "pente");
  const memberPrivacyGroup = await penteFactory.newPrivacyGroup({
    members: [verifierNode1, verifierNode2],
    evmVersion: "shanghai",
    externalCallsEnabled: true,
  });

  if (!checkDeploy(memberPrivacyGroup)) return false;

  // Step 2: Deploy a smart contract within the privacy group
  logger.log("Deploying a smart contract to the privacy group...");
  const contractAddress = await memberPrivacyGroup.deploy({
    abi: storageJson.abi,
    bytecode: storageJson.bytecode,
    from: verifierNode1.lookup,
  });

  if (!contractAddress) {
    logger.error("Failed to deploy the contract. No address returned.");
    return false;
  }

  logger.log(`Contract deployed successfully! Address: ${contractAddress}`);

  // Step 3: Use the deployed contract for private storage
  const privateStorageContract = new PrivateStorage(
    memberPrivacyGroup,
    contractAddress
  );

  // Store a value in the contract
  const valueToStore = 125; // Example value to store
  logger.log(`Storing a value "${valueToStore}" in the contract...`);
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
  logger.log("Node1 retrieving the value from the contract...");
  const retrievedValueNode1 = await privateStorageContract.call({
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
