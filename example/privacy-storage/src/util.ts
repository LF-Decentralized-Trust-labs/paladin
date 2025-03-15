import { ITransactionReceipt } from "@lfdecentralizedtrust-labs/paladin-sdk";

const logger = console;

export interface DeployedContract {
  address: string;
}

export function checkDeploy(
  contract: DeployedContract | undefined
): contract is DeployedContract {
  if (contract === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log(`Success! address: ${contract.address}`);
  return true;
}

export function checkReceipt(
  receipt: ITransactionReceipt | undefined
): receipt is ITransactionReceipt {
  if (receipt === undefined) {
    logger.error("Failed!");
    return false;
  } else if (receipt.failureMessage !== undefined) {
    logger.error(`Failed: ${receipt.failureMessage}`);
    return false;
  }
  logger.log("Success!");
  return true;
}
