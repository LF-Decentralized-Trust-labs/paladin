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
  TransactionType,
  ZetoFactory,
} from "@lfdecentralizedtrust/paladin-sdk";
import { checkDeploy, checkReceipt } from "paladin-example-common";
import erc20Abi from "./zeto-abis/SampleERC20.json";
import * as fs from 'fs';
import * as path from 'path';
import { ContractData } from "./tests/data-persistence";
import { nodeConnections, getCachePath, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";

const logger = console;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
  const [paladin1, paladin2, paladin3] = clients;

  const [cbdcIssuer] = paladin1.getVerifiers(`centralbank@${nodeConnections[2].id}`);
  const [bank1] = paladin2.getVerifiers(`bank1@${nodeConnections[0].id}`);
  const [bank2] = paladin3.getVerifiers(`bank2@${nodeConnections[1].id}`);

  const mintAmounts = [100000, 100000];
  const transferAmount = 1000;
  const erc20MintAmount = 100000;
  const erc20ApproveAmount = 10000;
  const depositAmount = 10000;
  const withdrawAmount = 1000;

  // Deploy a Zeto token to represent cash (CBDC)
  logger.log(
    "Use case #1: Privacy-preserving CBDC token, using private minting..."
  );
  logger.log("- Deploying Zeto token...");
  const zetoFactory = new ZetoFactory(paladin3, "zeto");
  const zetoCBDC1 = await zetoFactory
    .newZeto(cbdcIssuer, {
      tokenName: "Zeto_AnonNullifier",
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(zetoCBDC1)) return false;

  // Issue some cash
  logger.log("- Issuing CBDC to bank1 and bank2 with private minting...");
  let receipt = await zetoCBDC1
    .mint(cbdcIssuer, {
      mints: [
        {
          to: bank1,
          amount: mintAmounts[0],
          data: "0x",
        },
        {
          to: bank2,
          amount: mintAmounts[1],
          data: "0x",
        },
      ],
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;
  let bank1Balance = await zetoCBDC1
    .using(paladin1)
    .balanceOf(bank1, { account: bank1.lookup });
  logger.log(
    `bank1 State: ${bank1Balance.totalBalance} units of cash, ${bank1Balance.totalStates} states, overflow: ${bank1Balance.overflow}`
  );
  let bank2Balance = await zetoCBDC1
    .using(paladin2)
    .balanceOf(bank2, { account: bank2.lookup });
  logger.log(
    `bank2 State: ${bank2Balance.totalBalance} units of cash, ${bank2Balance.totalStates} states, overflow: ${bank2Balance.overflow}`
  );

  // TODO: remove
  await new Promise((resolve) => setTimeout(resolve, 3000));

  // Transfer some cash from bank1 to bank2
  logger.log(
    "- Bank1 transferring CBDC to bank2 to pay for some asset trades ..."
  );
  receipt = await zetoCBDC1
    .using(paladin1)
    .transfer(bank1, {
      transfers: [
        {
          to: bank2,
          amount: transferAmount,
          data: "0x",
        },
      ],
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;
  
  // Add a small delay to ensure state is settled
  await new Promise((resolve) => setTimeout(resolve, 2000));
  
  bank1Balance = await zetoCBDC1
    .using(paladin1)
    .balanceOf(bank1, { account: bank1.lookup });
  logger.log(
    `bank1 State: ${bank1Balance.totalBalance} units of cash, ${bank1Balance.totalStates} states, overflow: ${bank1Balance.overflow}`
  );
  bank2Balance = await zetoCBDC1
    .using(paladin2)
    .balanceOf(bank2, { account: bank2.lookup });
  logger.log(
    `bank2 State: ${bank2Balance.totalBalance} units of cash, ${bank2Balance.totalStates} states, overflow: ${bank2Balance.overflow}`
  );
  logger.log("\nUse case #1 complete!\n");

  logger.log(
    "Use case #2: Privacy-preserving CBDC token, using public minting of an ERC20 token..."
  );
  logger.log("- Deploying Zeto token...");
  const zetoCBDC2 = await zetoFactory
    .newZeto(cbdcIssuer, {
      tokenName: "Zeto_AnonNullifier",
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(zetoCBDC2)) return false;

  logger.log("- Deploying ERC20 token to manage the CBDC supply publicly...");
  const erc20Address = await deployERC20(paladin3, cbdcIssuer);
  logger.log(`  ERC20 deployed at: ${erc20Address}`);

  logger.log("- Setting ERC20 to the Zeto token contract ...");
  const result2 = await zetoCBDC2
    .setERC20(cbdcIssuer, {
      erc20: erc20Address as string,
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(result2)) return false;

  logger.log("- Issuing CBDC to bank1 with public minting in ERC20...");
  await mintERC20(paladin3, cbdcIssuer, bank1, erc20Address!, erc20MintAmount);
  logger.log(
    "- Bank1 approve ERC20 balance for the Zeto token contract as spender, to prepare for deposit..."
  );
  await approveERC20(paladin1, bank1, zetoCBDC2.address, erc20Address!, erc20ApproveAmount);

  logger.log("- Bank1 deposit ERC20 balance to Zeto ...");
  const result4 = await zetoCBDC2
    .using(paladin1)
    .deposit(bank1, {
      amount: depositAmount,
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(result4)) return false;
  const bank1BalanceAfterDeposit = await zetoCBDC2
    .using(paladin1)
    .balanceOf(bank1, { account: bank1.lookup });
  logger.log(
    `bank1 State: ${bank1Balance.totalBalance} units of cash, ${bank1Balance.totalStates} states, overflow: ${bank1Balance.overflow}`
  );

  // Transfer some cash from bank1 to bank2
  logger.log(
    "- Bank1 transferring CBDC to bank2 to pay for some asset trades ..."
  );
  receipt = await zetoCBDC2
    .using(paladin1)
    .transfer(bank1, {
      transfers: [
        {
          to: bank2,
          amount: transferAmount,
          data: "0x",
        },
      ],
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;
  
  // Add a small delay to ensure state is settled
  await new Promise((resolve) => setTimeout(resolve, 2000));
  
  const bank1BalanceUseCase2 = await zetoCBDC2
    .using(paladin1)
    .balanceOf(bank1, { account: bank1.lookup });
  logger.log(
    `bank1 State: ${bank1BalanceUseCase2.totalBalance} units of cash, ${bank1BalanceUseCase2.totalStates} states, overflow: ${bank1BalanceUseCase2.overflow}`
  );
  const bank2BalanceUseCase2 = await zetoCBDC2
    .using(paladin2)
    .balanceOf(bank2, { account: bank2.lookup });
  logger.log(
    `bank2 State: ${bank2BalanceUseCase2.totalBalance} units of cash, ${bank2BalanceUseCase2.totalStates} states, overflow: ${bank2BalanceUseCase2.overflow}`
  );

  logger.log("- Bank1 withdraws Zeto back to ERC20 balance ...");
  const result5 = await zetoCBDC2
    .using(paladin1)
    .withdraw(bank1, {
      amount: withdrawAmount,
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(result5)) return false;

  // Add a small delay to ensure state is settled
  await new Promise((resolve) => setTimeout(resolve, 2000));

  const finalBalanceBank1 = await zetoCBDC2
    .using(paladin1)
    .balanceOf(bank1, { account: bank1.lookup });
  const finalBalanceBank2 = await zetoCBDC2
    .using(paladin2)
    .balanceOf(bank2, { account: bank2.lookup });

  logger.log("\nUse case #2 complete!");

  // Save contract data to file for later use
  const contractData : ContractData= {
    zetoCBDC1Address: zetoCBDC1.address,
    zetoCBDC2Address: zetoCBDC2.address,
    erc20Address: erc20Address!,
    tokenName: "Zeto_AnonNullifier",
    useCase1: {
      mintAmounts: mintAmounts, // bank1, bank2
      transferAmount: transferAmount,
      finalBalances: {
        bank1: {
          totalBalance: bank1Balance.totalBalance,
          totalStates: bank1Balance.totalStates,
          overflow: bank1Balance.overflow
        },
        bank2: {
          totalBalance: bank2Balance.totalBalance,
          totalStates: bank2Balance.totalStates,
          overflow: bank2Balance.overflow
        }
      }
    },
    useCase2: {
      erc20MintAmount: erc20MintAmount,
      erc20ApproveAmount: erc20ApproveAmount,
      depositAmount: depositAmount,
      transferAmount: transferAmount,
      withdrawAmount: withdrawAmount,
      finalBalances: {
        bank1: {
          totalBalance: finalBalanceBank1.totalBalance,
          totalStates: finalBalanceBank1.totalStates,
          overflow: finalBalanceBank1.overflow
        },
        bank2: {
          totalBalance: finalBalanceBank2.totalBalance,
          totalStates: finalBalanceBank2.totalStates,
          overflow: finalBalanceBank2.overflow
        }
      }
    },
    cbdcIssuer: cbdcIssuer.lookup,
    bank1: bank1.lookup,
    bank2: bank2.lookup,
    timestamp: new Date().toISOString()
  };

  // Use command-line argument for data directory if provided, otherwise use default
  const dataDir = getCachePath();
  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }

  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const dataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(dataFile, JSON.stringify(contractData, null, 2));
  logger.log(`Contract data saved to ${dataFile}`);

  return true;
}

async function deployERC20(
  paladin: PaladinClient,
  cbdcIssuer: PaladinVerifier
): Promise<string | undefined> {
  const txId1 = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    from: cbdcIssuer.lookup,
    data: {
      initialOwner: await cbdcIssuer.address(),
    },
    function: "",
    abi: erc20Abi.abi,
    bytecode: erc20Abi.bytecode,
  });
  const result1 = await paladin.pollForReceipt(txId1, DEFAULT_POLL_TIMEOUT);
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
  const txId2 = await paladin.ptx.sendTransaction({
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
  const result3 = await paladin.pollForReceipt(txId2, DEFAULT_POLL_TIMEOUT);
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
  const txID1 = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: erc20Abi.abi,
    function: "approve",
    to: erc20Address,
    from: from.lookup,
    data: { value: amount, spender },
  });
  const result1 = await paladin.pollForReceipt(txID1, DEFAULT_POLL_TIMEOUT);
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
