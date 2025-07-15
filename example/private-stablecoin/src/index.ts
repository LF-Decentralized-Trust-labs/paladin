import PaladinClient, {
  PaladinVerifier,
  TransactionType,
  ZetoFactory,
  algorithmZetoSnarkBJJ,
  IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import { checkDeploy, checkReceipt } from "paladin-example-common";
import erc20Abi from "./abis/SampleERC20.json";
import kycAbi from "./abis/IZetoKyc.json";
import { buildBabyjub } from "circomlibjs";

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

async function getBabyjubPublicKey(
  verifier: PaladinVerifier
): Promise<string[]> {
  const pubKeyStr = await verifier.resolve(
    algorithmZetoSnarkBJJ("zeto") as any,
    IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X as any
  );

  if (!pubKeyStr || pubKeyStr === "0x0" || pubKeyStr === "0x") {
    throw new Error(`No babyjub key available for ${verifier.lookup}`);
  }

  const cleanHex = pubKeyStr.startsWith("0x") ? pubKeyStr.slice(2) : pubKeyStr;
  const compressedBytes = Buffer.from(cleanHex, "hex");

  if (compressedBytes.length !== 32) {
    throw new Error(
      `Invalid key length for ${verifier.lookup}: expected 32 bytes, got ${compressedBytes.length}`
    );
  }

  const babyJub = await buildBabyjub();
  const publicKey = babyJub.unpackPoint(compressedBytes);

  if (!publicKey || publicKey.length < 2) {
    throw new Error(
      `Failed to unpack babyjub key for ${verifier.lookup}: invalid point`
    );
  }

  return [babyJub.F.toString(publicKey[0]), babyJub.F.toString(publicKey[1])];
}

async function deployERC20(
  paladin: PaladinClient,
  issuer: PaladinVerifier
): Promise<string | undefined> {
  const txId = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,
    from: issuer.lookup,
    data: {
      initialOwner: await issuer.address(),
    },
    function: "",
    abi: erc20Abi.abi,
    bytecode: erc20Abi.bytecode,
  });
  const result = await paladin.pollForReceipt(txId, 5000);
  if (!checkReceipt(result)) {
    throw new Error("Failed to deploy ERC20 token");
  }
  return result.contractAddress;
}

async function mintERC20(
  paladin: PaladinClient,
  issuer: PaladinVerifier,
  recipient: PaladinVerifier,
  erc20Address: string,
  amount: number
): Promise<void> {
  const txId = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,
    from: issuer.lookup,
    to: erc20Address,
    data: {
      amount: amount,
      to: await recipient.address(),
    },
    function: "mint",
    abi: erc20Abi.abi,
  });
  const result = await paladin.pollForReceipt(txId, 5000);
  if (!checkReceipt(result)) {
    throw new Error("Failed to mint ERC20 tokens");
  }
}

async function approveERC20(
  paladin: PaladinClient,
  from: PaladinVerifier,
  spender: string,
  erc20Address: string,
  amount: number
): Promise<void> {
  const txId = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: erc20Abi.abi,
    function: "approve",
    to: erc20Address,
    from: from.lookup,
    data: { value: amount, spender },
  });
  const result = await paladin.pollForReceipt(txId, 5000);
  if (!checkReceipt(result)) {
    throw new Error("Failed to approve ERC20 transfer");
  }
}

async function getERC20Balance(
  paladin: PaladinClient,
  owner: PaladinVerifier,
  erc20Address: string
): Promise<number> {
  // Note: For simplicity in this example, we'll use a simple approach
  // In production, you would use a proper balance query method
  try {
    const result = await paladin.call({
      type: TransactionType.PUBLIC,
      abi: erc20Abi.abi,
      function: "balanceOf",
      to: erc20Address,
      from: owner.lookup,
      data: { account: await owner.address() },
    });
    return parseInt(result[0]?.toString());
  } catch (error) {
    // If balance query fails, return 0 for demo purposes
    console.warn(`Failed to get ERC20 balance: ${error}`);
    return 0;
  }
}

async function main(): Promise<boolean> {
  // Generate unique identity names for this run to avoid Merkle tree conflicts
  const runId = Math.random().toString(36).substring(2, 8);
  logger.log(`Using run ID: ${runId} for unique identities`);

  // Get verifiers for the financial institution and clients with unique names
  const [financialInstitution] = paladin1.getVerifiers(`bank-${runId}@node1`);
  const [clientA] = paladin2.getVerifiers(`client-a-${runId}@node2`);
  const [clientB] = paladin3.getVerifiers(`client-b-${runId}@node3`);

  logger.log("=== Private Stablecoin with KYC and Deposit/Withdraw ===");
  logger.log(
    "This example demonstrates a private stablecoin that exists as both"
  );
  logger.log(
    "a public ERC20 token and a private Zeto token with KYC compliance,"
  );
  logger.log(
    "showcasing deposit/withdraw functionality for privacy preservation.\n"
  );

  // === 1. DEPLOY CONTRACTS ===
  logger.log("1. Deploying contracts...");

  // Deploy the private stablecoin using Zeto_AnonNullifierKyc
  logger.log("   - Deploying Zeto private stablecoin with KYC capabilities...");
  const zetoFactory = new ZetoFactory(paladin1, "zeto");
  const privateStablecoin = await zetoFactory
    .newZeto(financialInstitution, {
      tokenName: "Zeto_AnonNullifierKyc",
    })
    .waitForDeploy();
  if (!checkDeploy(privateStablecoin)) return false;
  logger.log(
    `     ✓ Private stablecoin deployed at: ${privateStablecoin.address}`
  );

  // Deploy public ERC20 stablecoin
  logger.log("   - Deploying public ERC20 stablecoin contract...");
  const publicStablecoinAddress = await deployERC20(
    paladin1,
    financialInstitution
  );
  if (!publicStablecoinAddress) return false;
  logger.log(
    `     ✓ Public stablecoin deployed at: ${publicStablecoinAddress}`
  );

  // Connect the ERC20 to the Zeto contract for deposit/withdraw
  logger.log("   - Connecting ERC20 to Zeto contract...");
  const setERC20Receipt = await privateStablecoin
    .setERC20(financialInstitution, {
      erc20: publicStablecoinAddress,
    })
    .waitForReceipt();
  if (!checkReceipt(setERC20Receipt)) return false;
  logger.log("     ✓ ERC20 connected to Zeto contract\n");

  // === 2. KYC REGISTRATION ===
  logger.log(
    "2. Financial institution registering clients for KYC compliance..."
  );

  // Register Financial Institution itself for KYC
  logger.log("   - Registering Financial Institution for KYC...");
  const bankPublicKey = await getBabyjubPublicKey(financialInstitution);
  let kycTxId = await paladin1.sendTransaction({
    type: TransactionType.PUBLIC,
    from: financialInstitution.lookup,
    to: privateStablecoin.address,
    data: {
      publicKey: bankPublicKey,
      data: "0x", // KYC compliance data could go here
    },
    function: "register",
    abi: kycAbi.abi,
  });
  let kycReceipt = await paladin1.pollForReceipt(kycTxId, 5000);
  if (!checkReceipt(kycReceipt)) return false;
  logger.log("     ✓ Financial Institution registered for KYC");

  // Register Client A for KYC
  logger.log("   - Registering Client A for KYC...");
  const clientAPublicKey = await getBabyjubPublicKey(clientA);
  kycTxId = await paladin1.sendTransaction({
    type: TransactionType.PUBLIC,
    from: financialInstitution.lookup,
    to: privateStablecoin.address,
    data: {
      publicKey: clientAPublicKey,
      data: "0x", // KYC compliance data could go here
    },
    function: "register",
    abi: kycAbi.abi,
  });
  kycReceipt = await paladin1.pollForReceipt(kycTxId, 5000);
  if (!checkReceipt(kycReceipt)) return false;
  logger.log("     ✓ Client A registered for KYC");

  // Register Client B for KYC
  logger.log("   - Registering Client B for KYC...");
  const clientBPublicKey = await getBabyjubPublicKey(clientB);
  kycTxId = await paladin1.sendTransaction({
    type: TransactionType.PUBLIC,
    from: financialInstitution.lookup,
    to: privateStablecoin.address,
    data: {
      publicKey: clientBPublicKey,
      data: "0x", // KYC compliance data could go here
    },
    function: "register",
    abi: kycAbi.abi,
  });
  kycReceipt = await paladin1.pollForReceipt(kycTxId, 5000);
  if (!checkReceipt(kycReceipt)) return false;
  logger.log("     ✓ Client B registered for KYC\n");

  // === 3. MINT PUBLIC STABLECOINS ===
  logger.log("3. Financial institution minting public stablecoins...");

  // Mint public stablecoins to Client A
  logger.log("   - Minting 100,000 public stablecoins to Client A...");
  await mintERC20(
    paladin1,
    financialInstitution,
    clientA,
    publicStablecoinAddress,
    100000
  );
  let clientAPublicBalance = await getERC20Balance(
    paladin2,
    clientA,
    publicStablecoinAddress
  );
  logger.log(
    `     ✓ Client A public balance: ${clientAPublicBalance} stablecoins`
  );

  // Mint public stablecoins to Client B
  logger.log("   - Minting 50,000 public stablecoins to Client B...");
  await mintERC20(
    paladin1,
    financialInstitution,
    clientB,
    publicStablecoinAddress,
    50000
  );
  let clientBPublicBalance = await getERC20Balance(
    paladin3,
    clientB,
    publicStablecoinAddress
  );
  logger.log(
    `     ✓ Client B public balance: ${clientBPublicBalance} stablecoins\n`
  );

  // === 4. DEPOSIT: PUBLIC TO PRIVATE ===
  logger.log("4. Client A depositing public stablecoins for privacy...");

  // Client A approves Zeto contract to spend their ERC20 tokens
  logger.log("   - Client A approving Zeto contract to spend 75,000 tokens...");
  await approveERC20(
    paladin2,
    clientA,
    privateStablecoin.address,
    publicStablecoinAddress,
    75000
  );
  logger.log("     ✓ Approval granted");

  // Client A deposits ERC20 tokens to get private Zeto tokens
  logger.log("   - Client A depositing 75,000 tokens to get private tokens...");
  const depositReceipt = await privateStablecoin
    .using(paladin2)
    .deposit(clientA, {
      amount: 75000,
    })
    .waitForReceipt();
  if (!checkReceipt(depositReceipt)) return false;
  logger.log(
    "     ✓ Deposit successful - public tokens converted to private tokens"
  );

  // Check balances after deposit
  clientAPublicBalance = await getERC20Balance(
    paladin2,
    clientA,
    publicStablecoinAddress
  );
  let clientAPrivateBalance = await privateStablecoin
    .using(paladin2)
    .balanceOf(clientA, {
      account: clientA.lookup,
    });
  logger.log(`     ✓ Client A public balance: ${clientAPublicBalance} tokens`);
  logger.log(
    `     ✓ Client A private balance: ${clientAPrivateBalance.totalBalance} tokens (${clientAPrivateBalance.totalStates} states)\n`
  );

  // Brief pause for blockchain settlement
  await new Promise((resolve) => setTimeout(resolve, 2000));

  // === 5. PRIVATE TRANSFER ===
  logger.log("5. Client A making a private transfer to Client B...");

  const transferReceipt = await privateStablecoin
    .using(paladin2)
    .transfer(clientA, {
      transfers: [
        {
          to: clientB,
          amount: 25000, // Transfer 25,000 private tokens
          data: "0x",
        },
      ],
    })
    .waitForReceipt();
  if (!checkReceipt(transferReceipt)) return false;
  logger.log("     ✓ Private transfer successful");
  logger.log("     ✓ Transfer amount and parties remain private");

  // Check private balances after transfer
  clientAPrivateBalance = await privateStablecoin
    .using(paladin2)
    .balanceOf(clientA, {
      account: clientA.lookup,
    });
  let clientBPrivateBalance = await privateStablecoin
    .using(paladin3)
    .balanceOf(clientB, {
      account: clientB.lookup,
    });
  logger.log(
    `     ✓ Client A private balance: ${clientAPrivateBalance.totalBalance} tokens (${clientAPrivateBalance.totalStates} states)`
  );
  logger.log(
    `     ✓ Client B private balance: ${clientBPrivateBalance.totalBalance} tokens (${clientBPrivateBalance.totalStates} states)\n`
  );

  // Brief pause for blockchain settlement
  await new Promise((resolve) => setTimeout(resolve, 2000));

  // === 6. WITHDRAW: PRIVATE TO PUBLIC ===
  logger.log("6. Client B withdrawing private tokens back to public...");

  const withdrawReceipt = await privateStablecoin
    .using(paladin3)
    .withdraw(clientB, {
      amount: 15000, // Withdraw 15,000 tokens
    })
    .waitForReceipt();
  if (!checkReceipt(withdrawReceipt)) return false;
  logger.log(
    "     ✓ Withdrawal successful - private tokens converted back to public"
  );

  // Check final balances
  clientBPublicBalance = await getERC20Balance(
    paladin3,
    clientB,
    publicStablecoinAddress
  );
  clientBPrivateBalance = await privateStablecoin
    .using(paladin3)
    .balanceOf(clientB, {
      account: clientB.lookup,
    });
  logger.log(`     ✓ Client B public balance: ${clientBPublicBalance} tokens`);
  logger.log(
    `     ✓ Client B private balance: ${clientBPrivateBalance.totalBalance} tokens (${clientBPrivateBalance.totalStates} states)\n`
  );

  // === 7. FINAL SUMMARY ===
  logger.log("7. Final balance summary...");

  clientAPublicBalance = await getERC20Balance(
    paladin2,
    clientA,
    publicStablecoinAddress
  );
  clientAPrivateBalance = await privateStablecoin
    .using(paladin2)
    .balanceOf(clientA, {
      account: clientA.lookup,
    });

  logger.log("   Client A:");
  logger.log(`     - Public balance: ${clientAPublicBalance} stablecoins`);
  logger.log(
    `     - Private balance: ${clientAPrivateBalance.totalBalance} stablecoins (${clientAPrivateBalance.totalStates} states)`
  );

  logger.log("   Client B:");
  logger.log(`     - Public balance: ${clientBPublicBalance} stablecoins`);
  logger.log(
    `     - Private balance: ${clientBPrivateBalance.totalBalance} stablecoins (${clientBPrivateBalance.totalStates} states)`
  );

  logger.log("\n=== Private Stablecoin with KYC Example Complete ===");
  logger.log("✓ Successfully demonstrated:");
  logger.log("  - Dual public/private stablecoin system with KYC compliance");
  logger.log("  - Financial institution-managed KYC registration");
  logger.log("  - Seamless deposit (public → private) functionality");
  logger.log(
    "  - Privacy-preserving transfers with KYC verification using zero-knowledge proofs"
  );
  logger.log("  - Seamless withdraw (private → public) functionality");
  logger.log("  - Flexible liquidity between public and private domains");
  logger.log("  - Enterprise-grade privacy with regulatory compliance");

  return true;
}

// Export for potential testing
export { main };

// Run if called directly
if (require.main === module) {
  main()
    .then((success) => {
      if (success) {
        logger.log("\n🎉 Private stablecoin example completed successfully!");
        process.exit(0);
      } else {
        logger.error("\n❌ Private stablecoin example failed!");
        process.exit(1);
      }
    })
    .catch((error: unknown) => {
      logger.error("\n💥 Private stablecoin example crashed:", error);
      process.exit(1);
    });
}
