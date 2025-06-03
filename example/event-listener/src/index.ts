import PaladinClient, {
  ITransactionReceipt,
  PaladinWebSocketClient,
  PenteFactory,
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { nanoid } from "nanoid";
import { checkDeploy } from "paladin-example-common";
import helloWorldJson from "./abis/HelloWorld.json";

const logger = console;

const paladin = new PaladinClient({ url: "http://127.0.0.1:31548" });

async function main(): Promise<boolean> {
  const [verifierNode1] = paladin.getVerifiers("member@node1");

  // Create a privacy group for Node1 alone
  logger.log("Creating a privacy group for Node1...");
  const penteFactory = new PenteFactory(paladin, "pente");
  const memberPrivacyGroup = await penteFactory.newPrivacyGroup({
    members: [verifierNode1],
    evmVersion: "shanghai",
    externalCallsEnabled: true,
  });
  if (!checkDeploy(memberPrivacyGroup)) return false;

  // Deploy a smart contract within the privacy group
  logger.log("Deploying a smart contract to the privacy group...");
  const contractAddress = await memberPrivacyGroup.deploy({
    abi: helloWorldJson.abi,
    bytecode: helloWorldJson.bytecode,
    from: verifierNode1.lookup,
  });
  if (!contractAddress) {
    logger.error("Failed to deploy the contract. No address returned.");
    return false;
  }
  logger.log(`Contract deployed successfully! Address: ${contractAddress}`);

  // Check the latest sequence received
  const receipts = await paladin.queryTransactionReceipts({
    limit: 1,
    sort: ["-sequence"],
  });
  const lastSequence = receipts[0].sequence;
  logger.log(`Last sequence received: ${lastSequence}`);

  // Create a new listener (deleting one if it already exists)
  logger.log("Creating a receipt listener...");
  try {
    await paladin.deleteReceiptListener("example-event-listener");
  } catch (err) {
    // do nothing
  }
  await paladin.createReceiptListener({
    name: "example-event-listener",
    filters: {
      type: TransactionType.PRIVATE,
      domain: "pente",
      sequenceAbove: lastSequence,
    },
    options: {
      domainReceipts: true,
      incompleteStateReceiptBehavior: "block_contract",
    },
  });

  const name = nanoid(10);
  let received = false;

  // Create a websocket client for node1
  const processReceipt = async (receipt: ITransactionReceipt) => {
    if (receipt.domain === undefined) {
      return;
    }
    const domainReceipt = await paladin.getDomainReceipt(
      receipt.domain,
      receipt.id
    );
    if (domainReceipt !== undefined && "receipt" in domainReceipt) {
      for (const log of domainReceipt.receipt.logs ?? []) {
        const decoded = await paladin.decodeEvent(log.topics, log.data);
        const message = decoded?.data?.message;
        if (message?.indexOf(name) !== -1) {
          logger.log(`Received event data: ${JSON.stringify(decoded?.data)}`);
          received = true;
        }
      }
    }
  };
  const wsClient = new PaladinWebSocketClient(
    {
      url: "ws://127.0.0.1:31549",
      subscriptions: ["example-event-listener"],
    },
    async (sender, event) => {
      if (
        event.method === "ptx_subscription" &&
        "receipts" in event.params.result
      ) {
        for (const receipt of event.params.result.receipts) {
          // Process each transaction receipt
          await processReceipt(receipt);
        }

        // Ack the receipt batch
        sender.ack(event.params.subscription);
      }
    }
  );

  const sayHelloMethod = helloWorldJson.abi.find(
    (abi) => abi.name === "sayHello"
  );
  if (!sayHelloMethod) return false;

  // Call the sayHello function
  logger.log(`Saying hello to '${name}'...`);
  await memberPrivacyGroup.sendTransaction({
    methodAbi: sayHelloMethod,
    from: verifierNode1.lookup,
    to: contractAddress,
    data: { name },
  });

  // Wait to receive the receipt
  const attempts = 10;
  const delay = 100;
  for (let i = 0; i < attempts; i++) {
    if (received) break;
    await new Promise((resolve) => setTimeout(resolve, delay));
  }
  if (!received) {
    logger.error("Failed to receive the receipt.");
    return false;
  }

  await wsClient.close();

  return true;
}

if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      logger.error("Exiting due to an uncaught error:", err);
      process.exit(1);
    });
}
