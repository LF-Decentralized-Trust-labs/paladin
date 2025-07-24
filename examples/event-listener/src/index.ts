import PaladinClient, {
  ITransactionReceipt,
  PaladinWebSocketClient,
  PenteFactory,
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { nanoid } from "nanoid";
import { checkDeploy } from "paladin-example-common";
import helloWorldJson from "./abis/HelloWorld.json";
import * as fs from 'fs';
import * as path from 'path';
import { ContractData } from "./verify-deployed";

const logger = console;

const paladin = new PaladinClient({ url: "http://127.0.0.1:31548" });

async function main(): Promise<boolean> {
  const [verifierNode1] = paladin.getVerifiers("member@node1");

  // Create a privacy group for Node1 alone
  logger.log("Creating a privacy group for Node1...");
  const penteFactory = new PenteFactory(paladin, "pente");
  const memberPrivacyGroup = await penteFactory
    .newPrivacyGroup({
      members: [verifierNode1],
      evmVersion: "shanghai",
      externalCallsEnabled: true,
    })
    .waitForDeploy();
  if (!checkDeploy(memberPrivacyGroup)) return false;

  // Deploy a smart contract within the privacy group
  logger.log("Deploying a smart contract to the privacy group...");
  const deploy = memberPrivacyGroup.deploy({
    abi: helloWorldJson.abi,
    bytecode: helloWorldJson.bytecode,
    from: verifierNode1.lookup,
  });
  const receipt = await deploy.waitForReceipt();
  const contractAddress = await deploy.waitForDeploy();
  if (!receipt || !contractAddress) {
    logger.error("Failed to deploy the contract. No address returned.");
    return false;
  }
  const lastSequence = receipt.sequence;
  logger.log(
    `Contract deployed successfully! Address: ${contractAddress} Sequence: ${lastSequence}`
  );

  // Create a new listener (deleting one if it already exists)
  logger.log("Creating a receipt listener...");
  try {
    await paladin.ptx.deleteReceiptListener("example-event-listener");
  } catch (err) {
    // do nothing
  }
  await paladin.ptx.createReceiptListener({
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
  let receivedEventData: any = null;
  let receivedReceiptId: string | null = null;

  // Create a websocket client for node1
  const processReceipt = async (receipt: ITransactionReceipt) => {
    if (receipt.domain === undefined) {
      return;
    }
    logger.log(
      `Processing receipt ${receipt.id} (sequence: ${receipt.sequence})`
    );
    const domainReceipt = await paladin.ptx.getDomainReceipt(
      receipt.domain,
      receipt.id
    );
    if (domainReceipt !== undefined && "receipt" in domainReceipt) {
      // Skip if this receipt is not for our contract
      if (domainReceipt.receipt.to !== contractAddress) {
        logger.log(
          `Skipping receipt ${receipt.id} - not for our contract (to: ${domainReceipt.receipt.to})`
        );
        return;
      }
      logger.log(
        `Processing contract receipt ${receipt.id} (to: ${domainReceipt.receipt.to})`
      );
      for (const log of domainReceipt.receipt.logs ?? []) {
        const decoded = await paladin.ptx.decodeEvent(log.topics, log.data);
        const message = decoded?.data?.message;
        if (message?.indexOf(name) !== -1) {
          logger.log(`Received event data: ${JSON.stringify(decoded?.data)}`);
          received = true;
          receivedEventData = decoded?.data;
          receivedReceiptId = receipt.id;
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
        logger.log(
          `Received receipt batch with ${event.params.result.receipts.length} receipts`
        );
        for (const receipt of event.params.result.receipts) {
          // Process each transaction receipt
          await processReceipt(receipt);
        }

        // Ack the receipt batch
        logger.log(`Acknowledging receipt batch`);
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
  const txId = await memberPrivacyGroup.sendTransaction({
    methodAbi: sayHelloMethod,
    from: verifierNode1.lookup,
    to: contractAddress,
    data: { name },
  }).id;
  logger.log(`Transaction sent with ID: ${txId}`);

  // Wait to receive the receipt
  const attempts = 10;
  const delay = 500;
  for (let i = 0; i < attempts; i++) {
    if (received) {
      logger.log(
        `Successfully received welcome message after ${i + 1} attempts`
      );
      break;
    }
    await new Promise((resolve) => setTimeout(resolve, delay));
  }
  if (!received) {
    logger.error("Failed to receive the receipt.");
    return false;
  }

  // Add a small delay to ensure any additional receipts are processed
  logger.log(`Waiting for any additional receipts to be processed...`);
  await new Promise((resolve) => setTimeout(resolve, 500));

  await wsClient.close();

  // Save contract data to file for later use
  const contractData: ContractData = {
    privacyGroupId: memberPrivacyGroup.group.id,
    privacyGroupAddress: memberPrivacyGroup.address,
    contractAddress: contractAddress,
    listenerName: "example-event-listener",
    lastSequence: lastSequence,
    eventDetails: {
      name: name,
      receivedEventData: receivedEventData,
      receivedReceiptId: receivedReceiptId,
      transactionId: txId
    },
    listenerConfig: {
      type: TransactionType.PRIVATE,
      domain: "pente",
      sequenceAbove: lastSequence,
      domainReceipts: true,
      incompleteStateReceiptBehavior: "block_contract"
    },
    websocketConfig: {
      url: "ws://127.0.0.1:31549",
      subscriptions: ["example-event-listener"]
    },
    participant: {
      verifierNode1: verifierNode1.lookup
    },
    timestamp: new Date().toISOString()
  };

  const dataDir = path.join(__dirname, '..', 'data');
  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }

  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const dataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(dataFile, JSON.stringify(contractData, null, 2));
  logger.log(`Contract data saved to ${dataFile}`);

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
