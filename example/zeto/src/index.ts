import PaladinClient, {
  PaladinVerifier,
  TransactionType,
  ZetoFactory,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { checkDeploy, checkReceipt } from "paladin-example-common";
import erc20Abi from "./abis/SampleERC20.json";

const logger = console;

const paladin1 = new PaladinClient({
  url: "http://127.0.0.1:31548",
});
const paladin2 = new PaladinClient({
  url: "http://127.0.0.1:31648",
});
const paladin3 = new PaladinClient({
  url: "http://127.0.0.1:31748",
});

async function main(): Promise<boolean> {
  const [cbdcIssuer] = paladin1.getVerifiers("centralbank@node3");
  const [bank1] = paladin2.getVerifiers("bank1@node1");
  const [bank2] = paladin3.getVerifiers("bank2@node2");

  // Deploy a Zeto token to represent cash (CBDC)
  logger.log(
    "Use case #1: Privacy-preserving CBDC token, using private minting..."
  );
  logger.log("- Deploying Zeto token...");
  const zetoFactory = new ZetoFactory(paladin3, "zeto");
  const zetoCBDC1 = await zetoFactory.newZeto(cbdcIssuer, {
    tokenName: "Zeto_AnonNullifier",
  });
  if (!checkDeploy(zetoCBDC1)) return false;

  // Issue some cash
  logger.log("- Issuing CBDC to bank1 and bank2 with private minting...");
  let receipt = await zetoCBDC1.mint(cbdcIssuer, {
    mints: [
      {
        to: bank1,
        amount: 100000,
        data: "0x",
      },
      {
        to: bank2,
        amount: 100000,
        data: "0x",
      },
    ],
  });
  if (!checkReceipt(receipt)) return false;

  // TODO: remove
  await new Promise((resolve) => setTimeout(resolve, 3000));

  // Transfer some cash from bank1 to bank2
  logger.log(
    "- Bank1 transferring CBDC to bank2 to pay for some asset trades ..."
  );
  receipt = await zetoCBDC1.using(paladin1).transfer(bank1, {
    transfers: [
      {
        to: bank2,
        amount: 1000,
        data: "0x",
      },
    ],
  });
  if (!checkReceipt(receipt)) return false;
  logger.log("\nUse case #1 complete!\n");

  logger.log(
    "Use case #2: Privacy-preserving CBDC token, using public minting of an ERC20 token..."
  );
  logger.log("- Deploying Zeto token...");
  const zetoCBDC2 = await zetoFactory.newZeto(cbdcIssuer, {
    tokenName: "Zeto_AnonNullifier",
  });
  if (!checkDeploy(zetoCBDC2)) return false;

  logger.log("- Deploying ERC20 token to manage the CBDC supply publicly...");
  const erc20Address = await deployERC20(paladin3, cbdcIssuer);
  logger.log(`  ERC20 deployed at: ${erc20Address}`);

  logger.log("- Setting ERC20 to the Zeto token contract ...");
  const result2 = await zetoCBDC2.setERC20(cbdcIssuer, {
    erc20: erc20Address as string,
  });
  if (!checkReceipt(result2)) return false;

  logger.log("- Issuing CBDC to bank1 with public minting in ERC20...");
  await mintERC20(paladin3, cbdcIssuer, bank1, erc20Address!, 100000);

  logger.log(
    "- Bank1 approve ERC20 balance for the Zeto token contract as spender, to prepare for deposit..."
  );
  await approveERC20(paladin1, bank1, zetoCBDC2.address, erc20Address!, 10000);

  logger.log("- Bank1 deposit ERC20 balance to Zeto ...");
  const result4 = await zetoCBDC2.using(paladin1).deposit(bank1, {
    amount: 10000,
  });
  if (!checkReceipt(result4)) return false;

  // Transfer some cash from bank1 to bank2
  logger.log(
    "- Bank1 transferring CBDC to bank2 to pay for some asset trades ..."
  );
  receipt = await zetoCBDC2.using(paladin1).transfer(bank1, {
    transfers: [
      {
        to: bank2,
        amount: 1000,
        data: "0x",
      },
    ],
  });
  if (!checkReceipt(receipt)) return false;

  logger.log("- Bank1 withdraws Zeto back to ERC20 balance ...");
  const result5 = await zetoCBDC2.using(paladin1).withdraw(bank1, {
    amount: 1000,
  });
  if (!checkReceipt(result5)) return false;

  logger.log("\nUse case #2 complete!");

  return true;
}

async function deployERC20(
  paladin: PaladinClient,
  cbdcIssuer: PaladinVerifier
): Promise<string | undefined> {
  const txId1 = await paladin3.sendTransaction({
    type: TransactionType.PUBLIC,
    from: cbdcIssuer.lookup,
    data: {
      initialOwner: await cbdcIssuer.address(),
    },
    function: "",
    abi: erc20Abi.abi,
    bytecode: erc20Abi.bytecode,
  });
  const result1 = await paladin.pollForReceipt(txId1, 5000);
  if (!checkReceipt(result1)) {
    throw new Error("Failed to deploy ERC20 token");
  }
  const erc20Address = result1.contractAddress;
  return erc20Address;
}

async function mintERC20(
  paladin: PaladinClient,
  cbdcIssuer: PaladinVerifier,
  bank1: PaladinVerifier,
  erc20Address: string,
  amount: number
): Promise<void> {
  const txId2 = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,
    from: cbdcIssuer.lookup,
    to: erc20Address,
    data: {
      amount: amount,
      to: await bank1.address(),
    },
    function: "mint",
    abi: erc20Abi.abi,
  });
  const result3 = await paladin.pollForReceipt(txId2, 5000);
  if (!checkReceipt(result3)) {
    throw new Error("Failed to mint ERC20 tokens to bank1");
  }
}

async function approveERC20(
  paladin: PaladinClient,
  from: PaladinVerifier,
  spender: string,
  erc20Address: string,
  amount: number
): Promise<void> {
  // first approve the Zeto contract to draw the amount from our balance in the ERC20
  const txID1 = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: erc20Abi.abi,
    function: "approve",
    to: erc20Address,
    from: from.lookup,
    data: { value: amount, spender },
  });
  const result1 = await paladin.pollForReceipt(txID1, 5000);
  if (!checkReceipt(result1)) {
    throw new Error("Failed to approve transfer");
  }
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
