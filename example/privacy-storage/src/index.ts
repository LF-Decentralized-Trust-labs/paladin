import PaladinClient, {
  PenteFactory,
  newGroupSalt,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import storageJson from "./abis/Storage.json";
import { checkDeploy } from "./util";
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
  const memberPrivacyGroup = await penteFactory.newPrivacyGroup(verifierNode1, {
    group: {
      salt: newGroupSalt(), // Generate a new salt for the group
      members: [verifierNode1, verifierNode2], // Add members to the group
    },
    evmVersion: "shanghai",
    endorsementType: "group_scoped_identities",
    externalCallsEnabled: true,
  });

  if (!checkDeploy(memberPrivacyGroup)) return false;

  // Step 2: Deploy a smart contract within the privacy group
  logger.log("Deploying a smart contract to the privacy group...");
  const contractAddress = await memberPrivacyGroup.deploy(
    storageJson.abi, // ABI of the contract
    storageJson.bytecode, // Bytecode of the contract
    verifierNode1 // Deploying as Node1
  );

  if (!contractAddress) {
    logger.error("Failed to deploy the contract. No address returned.");
    return false;
  }

  logger.log(`Contract deployed successfully! Address: ${contractAddress}`);

  // Step 3: Use the deployed contract for private storage
  const privateStorageContract = new PrivateStorage(memberPrivacyGroup, contractAddress);

  // Store a value in the contract
  logger.log("Storing a value (125) in the contract...");
  const storeTx = await privateStorageContract.invoke(verifierNode1, "store", { num: 125 });
  logger.log("Value stored successfully! Transaction hash:", storeTx?.transactionHash);

  // Retrieve the value as Node1
  logger.log("Node1 retrieving the value from the contract...");
  const retrievedValueNode1 = await privateStorageContract.call(verifierNode1, "retrieve", []);
  logger.log("Node1 retrieved the value successfully:", retrievedValueNode1["0"]);

  // Retrieve the value as Node2
  logger.log("Node2 retrieving the value from the contract...");
  const retrievedValueNode2 = await privateStorageContract
    .using(paladinNode2)
    .call(verifierNode2, "retrieve", []);
  logger.log("Node2 retrieved the value successfully:", retrievedValueNode2["0"]);

  // Attempt to retrieve the value as Node3 (outsider)
  try {
    logger.log("Node3 (outsider) attempting to retrieve the value...");
    await privateStorageContract.using(paladinNode3).call(verifierNode3, "retrieve", []);
    logger.error("Node3 (outsider) should not have access to the privacy group!");
    return false;
  } catch (error) {
    logger.info("Node3 (outsider) cannot retrieve the data from the privacy group. Access denied.");
  }

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