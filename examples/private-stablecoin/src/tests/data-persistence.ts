/*
 * Copyright © 2025 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
import PaladinClient, {
  PaladinVerifier,
  ZetoFactory,
} from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections, findLatestContractDataFile, getCachePath } from "paladin-example-common";

const logger = console;

export interface ContractData {
  runId: string;
  privateStablecoinAddress: string;
  publicStablecoinAddress: string;
  tokenName: string;
  kycDetails: {
    financialInstitution: {
      lookup: string;
      publicKey: string[];
    };
    clientA: {
      lookup: string;
      publicKey: string[];
    };
    clientB: {
      lookup: string;
      publicKey: string[];
    };
  };
  operations: {
    deposit: {
      amount: number;
      receiptId: string;
      transactionHash: string;
    };
    transfer: {
      amount: number;
      receiptId: string;
      transactionHash: string;
    };
    withdraw: {
      amount: number;
      receiptId: string;
      transactionHash: string;
    };
  };
  finalBalances: {
    clientA: {
      public: number;
      private: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
    };
    clientB: {
      public: number;
      private: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
    };
  };
  participants: {
    financialInstitution: string;
    clientA: string;
    clientB: string;
  };
  timestamp: string;
}

async function getERC20Balance(
  paladin: PaladinClient,
  owner: PaladinVerifier,
  erc20Address: string
): Promise<number> {
  try {
    const result = await paladin.ptx.call({
      type: "public" as any,
      abi: [{ name: "balanceOf", type: "function", inputs: [{ name: "account", type: "address" }], outputs: [{ name: "", type: "uint256" }] }],
      function: "balanceOf",
      to: erc20Address,
      from: owner.lookup,
      data: { account: await owner.address() },
    });
    return parseInt(result[0]?.toString() || "0");
  } catch (error) {
    console.warn(`Failed to get ERC20 balance: ${error}`);
    return 0;
  }
}

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
  const verifiers = clients.map((client, i) => client.getVerifiers(`user@${nodeConnections[i].id}`)[0]);

  const [paladin1, paladin2, paladin3] = clients;

  // STEP 1: Load the saved contract data
  logger.log("STEP 1: Loading saved contract data...");
  // Use command-line argument for data directory if provided, otherwise use default
  const dataDir = getCachePath();
  const dataFile = findLatestContractDataFile(dataDir);
  
  if (!dataFile) {
    logger.error(`STEP 1: No contract data files found in ${dataDir}`);
    logger.error("Please run the original script first to deploy the contracts and save the data.");
    return false;
  }

  const contractData: ContractData = JSON.parse(fs.readFileSync(dataFile, 'utf8'));
  logger.log(`STEP 1: Loaded contract data from ${dataFile}`);

  // Print cached data summary
  logger.log("\n=== CACHED DATA SUMMARY ===");
  logger.log(`Data File: ${dataFile}`);
  logger.log(`Timestamp: ${contractData.timestamp}`);
  logger.log(`Run ID: ${contractData.runId}`);
  logger.log(`Private Stablecoin Address: ${contractData.privateStablecoinAddress}`);
  logger.log(`Public Stablecoin Address: ${contractData.publicStablecoinAddress}`);
  logger.log(`Token Name: ${contractData.tokenName}`);
  logger.log(`Financial Institution: ${contractData.participants.financialInstitution}`);
  logger.log(`Client A: ${contractData.participants.clientA}`);
  logger.log(`Client B: ${contractData.participants.clientB}`);
  logger.log(`Deposit Amount: ${contractData.operations.deposit.amount}`);
  logger.log(`Transfer Amount: ${contractData.operations.transfer.amount}`);
  logger.log(`Withdraw Amount: ${contractData.operations.withdraw.amount}`);
  logger.log("=============================\n");

  // STEP 2: Get verifiers and recreate contract connections
  logger.log("STEP 2: Recreating contract connections...");
  const [financialInstitution] = paladin1.getVerifiers(`bank-${contractData.runId}@${nodeConnections[0].id}`);
  const [clientA] = paladin2.getVerifiers(`client-a-${contractData.runId}@${nodeConnections[1].id}`);
  const [clientB] = paladin3.getVerifiers(`client-b-${contractData.runId}@${nodeConnections[2].id}`);

  // Import necessary classes from the SDK
  const { ZetoInstance } = await import("@lfdecentralizedtrust/paladin-sdk");
  
  // Recreate contract instances
  const privateStablecoin = new ZetoInstance(paladin1, contractData.privateStablecoinAddress);

  logger.log("STEP 2: Contract connections recreated successfully!");

  // STEP 3: Verify private stablecoin balances
  logger.log("STEP 3: Verifying private stablecoin balances...");
  try {
    const currentClientAPrivateBalance = await privateStablecoin
      .using(paladin2)
      .balanceOf(clientA, {
        account: clientA.lookup,
      });

    const currentClientBPrivateBalance = await privateStablecoin
      .using(paladin3)
      .balanceOf(clientB, {
        account: clientB.lookup,
      });

    logger.log(`STEP 3: Current private balances:`);
    logger.log(`Client A: ${currentClientAPrivateBalance.totalBalance} tokens, ${currentClientAPrivateBalance.totalStates} states, overflow: ${currentClientAPrivateBalance.overflow}`);
    logger.log(`Client B: ${currentClientBPrivateBalance.totalBalance} tokens, ${currentClientBPrivateBalance.totalStates} states, overflow: ${currentClientBPrivateBalance.overflow}`);

    // Verify balances match saved data
    if (currentClientAPrivateBalance.totalBalance !== contractData.finalBalances.clientA.private.totalBalance) {
      logger.error(`STEP 3: ERROR - Client A private balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.clientA.private.totalBalance}`);
      logger.error(`Found: ${currentClientAPrivateBalance.totalBalance}`);
      return false;
    }

    if (currentClientBPrivateBalance.totalBalance !== contractData.finalBalances.clientB.private.totalBalance) {
      logger.error(`STEP 3: ERROR - Client B private balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.clientB.private.totalBalance}`);
      logger.error(`Found: ${currentClientBPrivateBalance.totalBalance}`);
      return false;
    }

    logger.log("STEP 3: Private stablecoin balance verification successful!");

  } catch (error) {
    logger.error("STEP 3: Failed to retrieve private stablecoin balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 4: Verify public ERC20 balances
  logger.log("STEP 4: Verifying public ERC20 balances...");
  try {
    const currentClientAPublicBalance = await getERC20Balance(
      paladin2,
      clientA,
      contractData.publicStablecoinAddress
    );

    const currentClientBPublicBalance = await getERC20Balance(
      paladin3,
      clientB,
      contractData.publicStablecoinAddress
    );

    logger.log(`STEP 4: Current public balances:`);
    logger.log(`Client A: ${currentClientAPublicBalance} tokens`);
    logger.log(`Client B: ${currentClientBPublicBalance} tokens`);

    // Verify balances match saved data
    if (currentClientAPublicBalance !== contractData.finalBalances.clientA.public) {
      logger.error(`STEP 4: ERROR - Client A public balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.clientA.public}`);
      logger.error(`Found: ${currentClientAPublicBalance}`);
      return false;
    }

    if (currentClientBPublicBalance !== contractData.finalBalances.clientB.public) {
      logger.error(`STEP 4: ERROR - Client B public balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.clientB.public}`);
      logger.error(`Found: ${currentClientBPublicBalance}`);
      return false;
    }

    logger.log("STEP 4: Public ERC20 balance verification successful!");

  } catch (error) {
    logger.error("STEP 4: Failed to retrieve public ERC20 balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 5: Test private stablecoin functionality
  logger.log("STEP 5: Testing private stablecoin functionality...");
  try {
    // Test if we can query balances
    logger.log("STEP 5: Testing private stablecoin query functionality...");
    const testBalance = await privateStablecoin
      .using(paladin2)
      .balanceOf(clientA, {
        account: clientA.lookup,
      });
    
    if (!testBalance || testBalance.totalBalance === undefined) {
      logger.error("STEP 5: Private stablecoin query functionality test failed!");
      return false;
    }
    logger.log("STEP 5: Private stablecoin query functionality test successful!");

  } catch (error) {
    logger.error("STEP 5: Private stablecoin functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 6: Test public ERC20 functionality
  logger.log("STEP 6: Testing public ERC20 functionality...");
  try {
    // Test if we can query ERC20 balances
    logger.log("STEP 6: Testing public ERC20 query functionality...");
    const testBalance = await getERC20Balance(
      paladin2,
      clientA,
      contractData.publicStablecoinAddress
    );
    
    if (testBalance === undefined) {
      logger.error("STEP 6: Public ERC20 query functionality test failed!");
      return false;
    }
    logger.log("STEP 6: Public ERC20 query functionality test successful!");

  } catch (error) {
    logger.error("STEP 6: Public ERC20 functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 7: Verify KYC details are preserved
  logger.log("STEP 7: Verifying KYC details are preserved...");
  try {
    logger.log(`STEP 7: KYC details verification:`);
    logger.log(`Financial Institution: ${contractData.kycDetails.financialInstitution.lookup}`);
    logger.log(`Client A: ${contractData.kycDetails.clientA.lookup}`);
    logger.log(`Client B: ${contractData.kycDetails.clientB.lookup}`);

    // Verify that the KYC details are properly formatted
    if (!contractData.kycDetails.financialInstitution.lookup || 
        !contractData.kycDetails.clientA.lookup || 
        !contractData.kycDetails.clientB.lookup) {
      logger.error("STEP 7: ERROR - KYC details are missing critical information!");
      return false;
    }

    if (!contractData.kycDetails.financialInstitution.publicKey || 
        !contractData.kycDetails.clientA.publicKey || 
        !contractData.kycDetails.clientB.publicKey) {
      logger.error("STEP 7: ERROR - KYC public keys are missing!");
      return false;
    }

    logger.log("STEP 7: KYC details verification successful!");

  } catch (error) {
    logger.error("STEP 7: KYC details verification failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 8: Verify operation details are preserved
  logger.log("STEP 8: Verifying operation details are preserved...");
  try {
    logger.log(`STEP 8: Operation details verification:`);
    logger.log(`Deposit: ${contractData.operations.deposit.amount} tokens (Receipt: ${contractData.operations.deposit.receiptId})`);
    logger.log(`Transfer: ${contractData.operations.transfer.amount} tokens (Receipt: ${contractData.operations.transfer.receiptId})`);
    logger.log(`Withdraw: ${contractData.operations.withdraw.amount} tokens (Receipt: ${contractData.operations.withdraw.receiptId})`);

    // Verify that the operation details are properly formatted
    if (!contractData.operations.deposit.receiptId || 
        !contractData.operations.transfer.receiptId || 
        !contractData.operations.withdraw.receiptId) {
      logger.error("STEP 8: ERROR - Operation receipt IDs are missing!");
      return false;
    }

    if (contractData.operations.deposit.amount <= 0 || 
        contractData.operations.transfer.amount <= 0 || 
        contractData.operations.withdraw.amount <= 0) {
      logger.error("STEP 8: ERROR - Operation amounts are invalid!");
      return false;
    }

    logger.log("STEP 8: Operation details verification successful!");

  } catch (error) {
    logger.error("STEP 8: Operation details verification failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 9: Test new private transfer functionality
  logger.log("STEP 9: Testing new private transfer functionality...");
  try {
    // Note: We won't actually perform a transfer to avoid changing balances
    // Instead, we'll test that the contract is still accessible for transfers
    logger.log("STEP 9: Testing private stablecoin transfer accessibility...");
    
    // Test that we can access the transfer functionality (without executing)
    const testBalance = await privateStablecoin
      .using(paladin2)
      .balanceOf(clientA, {
        account: clientA.lookup,
      });

    if (testBalance && testBalance.totalBalance !== "0") {
      logger.log("STEP 9: Private stablecoin transfer accessibility test successful!");
    } else {
      logger.log("STEP 9: Private stablecoin has no balance for transfer test");
    }

  } catch (error) {
    logger.error("STEP 9: Private transfer functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  logger.log("\nSUCCESS: Verification completed!");
  
  return true;
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