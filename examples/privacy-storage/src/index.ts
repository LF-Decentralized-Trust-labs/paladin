import PaladinClient, {
  PenteFactory,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { checkDeploy } from "paladin-example-common";
import storageJson from "./abis/Storage.json";
import { PrivateStorage } from "./helpers/storage";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections } from "../../common/src/config";

const logger = console;

// ASCII art for Pente
const penteArt = `
██████╗   ███████╗  ███╗   ██╗  ████████╗  ███████╗
██╔══██╗  ██╔════╝  ████╗  ██║  ╚══██╔══╝  ██╔════╝
██████╔╝  █████╗    ██╔██╗ ██║     ██║     █████╗  
██╔═══╝   ██╔══╝    ██║╚██╗██║     ██║     ██╔══╝  
██║       ███████╗  ██║ ╚████║     ██║     ███████╗
╚═╝       ╚══════╝  ╚═╝  ╚═══╝     ╚═╝     ╚══════╝
`;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  logger.log(penteArt);
  logger.log("==================================================================================");
  logger.log("🛡️  Initializing Paladin Clients for Private Storage Demonstration 🛡️");
  logger.log("==================================================================================");

  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
  const [paladinNode1, paladinNode2, paladinNode3] = clients;

  const [verifierNode1] = paladinNode1.getVerifiers(`member@${nodeConnections[0].id}`);
  const [verifierNode2] = paladinNode2.getVerifiers(`member@${nodeConnections[1].id}`);
  const [verifierNode3] = paladinNode3.getVerifiers(`outsider@${nodeConnections[2].id}`);

  // Step 1: Create a privacy group for members
  logger.log("\n==================================================================================");
  logger.log("🤫 Step 1: Creating a privacy group for Node1 and Node2...");
  logger.log("==================================================================================");
  const penteFactory = new PenteFactory(paladinNode1, "pente");
  const memberPrivacyGroup = await penteFactory.newPrivacyGroup({
    members: [verifierNode1, verifierNode2],
    evmVersion: "shanghai",
    externalCallsEnabled: true,
  }).waitForDeploy();
  if (!checkDeploy(memberPrivacyGroup)) return false;

  logger.log(`✅ Privacy group created, ID: ${memberPrivacyGroup?.group.id}`);

  // Step 2: Deploy a smart contract within the privacy group
  logger.log("\n==================================================================================");
  logger.log("📜 Step 2: Deploying a smart contract to the privacy group...");
  logger.log("==================================================================================");
  const contractAddress = await memberPrivacyGroup.deploy({
    abi: storageJson.abi,
    bytecode: storageJson.bytecode,
    from: verifierNode1.lookup,
  }).waitForDeploy();
  if (!contractAddress) {
    logger.error("Failed to deploy the contract. No address returned.");
    return false;
  }

  logger.log(`✅ Contract deployed successfully! Address: ${contractAddress}`);

  // Step 3: Use the deployed contract for private storage
  logger.log("\n==================================================================================");
  logger.log("🔐 Step 3: Using the deployed contract for private storage");
  logger.log("==================================================================================");
  const privateStorageContract = new PrivateStorage(
    memberPrivacyGroup,
    contractAddress
  );

  // Store a value in the contract
  const valueToStore = 125; // Example value to store
  logger.log(`Storing a value "${valueToStore}" in the contract...`);
  const storeReceipt = await privateStorageContract.sendTransaction({
    from: verifierNode1.lookup,
    function: "store",
    data: { num: valueToStore },
  }).waitForReceipt(10000);
  logger.log(
    `✅ Value stored successfully! Transaction hash: ${storeReceipt?.transactionHash}`
  );

  // Retrieve the value as Node1
  logger.log("\n----------------------------------------------------------------------------------");
  logger.log("🔍 Node1 retrieving the value from the contract...");
  const retrievedValueNode1 = await privateStorageContract.call({
    from: verifierNode1.lookup,
    function: "retrieve",
  });
  logger.log(
    `✅ Node1 retrieved the value successfully: ${retrievedValueNode1["value"]}`
  );

  // Retrieve the value as Node2
  logger.log("\n----------------------------------------------------------------------------------");
  logger.log("🔍 Node2 retrieving the value from the contract...");
  const retrievedValueNode2 = await privateStorageContract
    .using(paladinNode2)
    .call({
      from: verifierNode2.lookup,
      function: "retrieve",
    });
  logger.log(
    `✅ Node2 retrieved the value successfully: ${retrievedValueNode2["value"]}`
  );

  // Attempt to retrieve the value as Node3 (outsider)
  logger.log("\n----------------------------------------------------------------------------------");
  logger.log("🚫 Node3 (outsider) attempting to retrieve the value...");
  try {
    await privateStorageContract.using(paladinNode3).call({
      from: verifierNode3.lookup,
      function: "retrieve",
    });
    logger.error(
      "❌ Node3 (outsider) should not have access to the privacy group!"
    );
    return false;
  } catch (error) {
    logger.log(
      "✅ Node3 (outsider) correctly denied access to the privacy group!"
    );
  }

  // Save contract data to file for later use
  const contractData = {
    privacyGroupId: memberPrivacyGroup?.group.id,
    contractAddress: contractAddress,
    storedValue: valueToStore,
    retrievedValueNode1: retrievedValueNode1["value"],
    retrievedValueNode2: retrievedValueNode2["value"],
    storeTransactionHash: storeReceipt?.transactionHash,
    node1Verifier: verifierNode1.lookup,
    node2Verifier: verifierNode2.lookup,
    node3Verifier: verifierNode3.lookup,
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
