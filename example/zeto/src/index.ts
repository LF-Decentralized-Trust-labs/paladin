import PaladinClient, {
  Algorithms,
  newTransactionId,
  ZetoFactory,
  TransactionType,
  Verifiers,
} from "paladin-sdk";
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

async function main() {
  const cbdcIssuer = "centralbank@node3";
  const bank1 = `bank1@node1`;
  const bank2 = `bank2@node2`;

  // Deploy a Zeto token to represent cash (CBDC)
  logger.log("Use case #1: Privacy-preserving CBDC token, using private minting...");
  logger.log("- Deploying Zeto token...");
  const zetoFactory = new ZetoFactory(paladin3, "zeto");
  const zetoCBDC1 = await zetoFactory.newZeto(cbdcIssuer, {
    tokenName: "Zeto_AnonNullifier",
  });
  if (zetoCBDC1 === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log(`  Zeto deployed at: ${zetoCBDC1.address}`);

  // Issue some cash
  logger.log("- Issuing CBDC to bank1 and bank2 with private minting...");
  let receipt = await zetoCBDC1.mint(cbdcIssuer, {
    mints: [
      {
        to: bank1,
        amount: 100000,
      },
      {
        to: bank2,
        amount: 100000,
      },
    ]
  });
  if (receipt === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log("  Success!");

  // Transfer some cash from bank1 to bank2
  logger.log("- Bank1 transferring CBDC to bank2 to pay for some asset trades ...");
  receipt = await zetoCBDC1.using(paladin1).transfer(bank1, {
    transfers: [
      {
        to: bank2,
        amount: 1000,
      },
    ]
  });
  if (receipt === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log("  Success!\n");

  logger.log("Use case #2: Privacy-preserving CBDC token, using public minting of an ERC20 token...");
  logger.log("- Deploying Zeto token...");
  const zetoCBDC2 = await zetoFactory.newZeto(cbdcIssuer, {
    tokenName: "Zeto_AnonNullifier",
  });
  if (zetoCBDC2 === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log(`  Zeto deployed at: ${zetoCBDC2.address}`);

  logger.log("- Deploying ERC20 token to manage the CBDC supply publicly...");
  const cbdcIssuerAddress = await paladin1.resolveVerifier(
    cbdcIssuer,
    Algorithms.ECDSA_SECP256K1,
    Verifiers.ETH_ADDRESS
  );

  const txId1 = await paladin3.sendTransaction({
    type: TransactionType.PUBLIC,
    from: cbdcIssuer,
    data: {
      "initialOwner": cbdcIssuerAddress,
    },
    function: "",
    abi: erc20Abi.abi,
    bytecode: erc20Abi.bytecode,
  });
  if (txId1 === undefined) {
    logger.error("Failed!");
    return;
  }
  const result1 = await paladin3.pollForReceipt(txId1, 5000);
  if (result1 === undefined) {
    logger.error("Failed!");
    return;
  }
  const erc20Address = result1.contractAddress;
  logger.log(`  ERC20 deployed at: ${erc20Address}`);

  logger.log("- Setting ERC20 to the Zeto token contract ...");
  const result2 = await zetoCBDC2.setERC20(cbdcIssuer, {
    _erc20: erc20Address as string
  });
  if (result2 === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log(`  ERC20 configured on the Zeto contract`);

  logger.log("- Issuing CBDC to bank1 with public minting in ERC20...");
  const bank1Address = await paladin1.resolveVerifier(
    bank1,
    Algorithms.ECDSA_SECP256K1,
    Verifiers.ETH_ADDRESS
  );
  const txId2 = await paladin3.sendTransaction({
    type: TransactionType.PUBLIC,
    from: cbdcIssuer,
    to: erc20Address,
    data: {
      "amount": 100000,
      "to": bank1Address,
    },
    function: "mint",
    abi: erc20Abi.abi,
  });
  if (txId2 === undefined) {
    logger.error("Failed!");
    return;
  }
  const result3 = await paladin3.pollForReceipt(txId2, 5000);
  if (result3 === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log("  Success!");

  logger.log("- Bank1 deposit ERC20 balance to Zeto ...");
  const result4 = await zetoCBDC2.using(paladin1).deposit(bank1, {
    amount: 10000
  });
  if (result4 === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log(`  Bank1 deposit successful`);

  // Transfer some cash from bank1 to bank2
  logger.log("- Bank1 transferring CBDC to bank2 to pay for some asset trades ...");
  receipt = await zetoCBDC2.using(paladin1).transfer(bank1, {
    transfers: [
      {
        to: bank2,
        amount: 1000,
      },
    ]
  });
  if (receipt === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log("  Success!");

  logger.log("- Bank1 withdraws Zeto back to ERC20 balance ...");
  const result5 = await zetoCBDC2.using(paladin1).withdraw(bank1, {
    amount: 1000
  });
  if (result5 === undefined) {
    logger.error("Failed!");
    return;
  }
  logger.log(`  Bank1 withdraw successful`);

  logger.log("\nSuccess!");
}

function getFunctionAbi(abi: any, functionName: string): any {
  return abi.find((element: any) => element.name === functionName);
}

if (require.main === module) {
  main().catch((err) => {
    console.error("Exiting with uncaught error");
    console.error(err);
    process.exit(1);
  });
}
