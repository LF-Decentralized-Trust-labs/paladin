import PaladinClient, {
  PenteFactory,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { checkDeploy } from "paladin-example-common";
import storageJson from "./abis/Storage.json";
import { PrivateStorage } from "./helpers/storage";

const logger = console;

// Initialize Paladin clients for three nodes
const paladinNode1 = new PaladinClient({ url: "http://127.0.0.1:31548" });

async function main(): Promise<boolean> {
  // Get verifiers for each node
  const [verifierNode1] = paladinNode1.getVerifiers("member@node1");

  const receipt = await paladinNode1.getTransactionReceipt(
    "165471fc-95e4-400d-b7c4-02428e170d67",
    true
  );

  console.log(verifierNode1.address);

  console.log("receipt: ", receipt);

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
