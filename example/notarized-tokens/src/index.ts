import PaladinClient, {
  NotoFactory,
} from "@lfdecentralizedtrust-labs/paladin-sdk";

const logger = console;

// Initialize Paladin clients for three nodes
const paladinClientNode1 = new PaladinClient({ url: "http://127.0.0.1:31548" });
const paladinClientNode2 = new PaladinClient({ url: "http://127.0.0.1:31648" });
const paladinClientNode3 = new PaladinClient({ url: "http://127.0.0.1:31748" });

async function main(): Promise<boolean> {
  // Retrieve verifiers for each node
  const [verifierNode1] = paladinClientNode1.getVerifiers("user@node1");
  const [verifierNode2] = paladinClientNode2.getVerifiers("user@node2");
  const [verifierNode3] = paladinClientNode3.getVerifiers("user@node3");

  // Step 1: Deploy a Noto token to represent cash
  logger.log("Step 1: Deploying a Noto cash token...");
  const notoFactory = new NotoFactory(paladinClientNode1, "noto");
  const cashToken = await notoFactory.newNoto(verifierNode1, {
    notary: verifierNode1,
    notaryMode: "basic"
  });
  if (!cashToken) {
    logger.error("Failed to deploy the Noto cash token!");
    return false;
  }
  logger.log("Noto cash token deployed successfully!");

  // Step 2: Mint cash tokens
  logger.log("Step 2: Minting 2000 units of cash to Node1...");
  const mintReceipt = await cashToken.mint(verifierNode1, {
    to: verifierNode1,
    amount: 2000,
    data: "0x",
  });
  if (!mintReceipt) {
    logger.error("Failed to mint cash tokens!");
    return false;
  }
  logger.log("Successfully minted 2000 units of cash to Node1!");

  // Step 3: Transfer cash to Node2
  logger.log("Step 3: Transferring 1000 units of cash from Node1 to Node2...");
  const transferToNode2 = await cashToken.transfer(verifierNode1, {
    to: verifierNode2,
    amount: 1000,
    data: "0x",
  });
  if (!transferToNode2) {
    logger.error("Failed to transfer cash to Node2!");
    return false;
  }
  logger.log("Successfully transferred 1000 units of cash to Node2!");

  // Step 4: Transfer cash to Node3 from Node2
  logger.log("Step 4: Transferring 800 units of cash from Node2 to Node3...");
  const transferToNode3 = await cashToken.using(paladinClientNode2).transfer(verifierNode2, {
    to: verifierNode3,
    amount: 800,
    data: "0x",
  });
  if (!transferToNode3) {
    logger.error("Failed to transfer cash to Node3!");
    return false;
  }
  logger.log("Successfully transferred 800 units of cash to Node3!");

  // All steps completed successfully
  logger.log("All operations completed successfully!");
  return true;
}

// Execute the main function if this file is run directly
if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1); // Exit with 0 for success, 1 for failure
    })
    .catch((err) => {
      logger.error("Exiting due to an uncaught error:", err);
      process.exit(1); // Exit with status 1 for any uncaught errors
    });
}
