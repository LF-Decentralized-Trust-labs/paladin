import { ITransactionReceipt } from "@lfdecentralizedtrust/paladin-sdk";

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

/**
 * Waits for a domain receipt to become available with exponential backoff.
 * This is useful when processing WebSocket events where domain receipts might not be immediately available.
 * 
 * @param paladin - The Paladin client instance
 * @param domain - The domain name
 * @param transactionId - The transaction ID to wait for
 * @param options - Configuration options for the wait behavior
 * @returns The domain receipt when available, or undefined if timeout is reached
 */
export async function waitForDomainReceipt(
  paladin: PaladinClient,
  domain: string,
  transactionId: string,
  options: {
    maxAttempts?: number;
    baseDelay?: number;
    logger?: Console;
  } = {}
): Promise<any> {
  const {
    maxAttempts = 20,
    baseDelay = 500,
    logger: customLogger = console
  } = options;

  let attempts = 0;
  
  while (attempts < maxAttempts) {
    try {
      const domainReceipt = await paladin.ptx.getDomainReceipt(
        domain,
        transactionId
      );
      if (domainReceipt) {
        return domainReceipt; // Success, return the receipt
      }
    } catch (error: any) {
      // Only retry on "not yet available" errors
      if (!error.message?.includes('not yet available') && !error.message?.includes('no state confirmations')) {
        customLogger.log(`Non-retryable error getting domain receipt: ${error.message}`);
        throw error; // Re-throw non-retryable errors
      }
    }
    
    attempts++;
    if (attempts < maxAttempts) {
      const delay = baseDelay * Math.pow(1.5, attempts - 1); // Exponential backoff
      customLogger.log(`Domain receipt not ready yet, waiting ${Math.round(delay)}ms (attempt ${attempts}/${maxAttempts})`);
      await new Promise(resolve => setTimeout(resolve, delay));
    }
  }
  
  customLogger.log(`Domain receipt still not available for ${transactionId} after ${maxAttempts} attempts`);
  return undefined;
}
