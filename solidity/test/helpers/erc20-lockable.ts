import type { Signer } from "ethers";
import { ethers } from "hardhat";
import type { ERC20Lockable } from "../../typechain-types";

export const ERC20_CREATE_LOCK_INPUTS_TYPE =
  "tuple(uint256 amount, address recipient)";

// ERC-8074 type string — must match ERC20Lockable.ERC20LockContentType
export const ERC20_LOCK_CONTENT_TYPE =
  "ERC20LockContent(uint256 amount,address recipient)";

export function encodeCreateLockInputs(
  amount: bigint,
  recipient: string,
): string {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    [ERC20_CREATE_LOCK_INPUTS_TYPE],
    [{ amount, recipient }],
  );
}

/**
 * Helper to create a lock in an ERC20Lockable token and get the lock ID
 * @param token The ERC20Lockable contract instance
 * @param owner The signer who will own the lock
 * @param amount The amount to lock (in wei)
 * @param recipient The address that will receive tokens when lock is spent
 * @returns Promise resolving to the lock ID as a hex string
 */
export async function createTokenLock(
  token: ERC20Lockable,
  owner: Signer,
  amount: bigint,
  recipient: string,
): Promise<string> {
  const createInputs = encodeCreateLockInputs(amount, recipient);

  const tx = await token
    .connect(owner)
    .createLock(createInputs, ethers.ZeroHash, ethers.ZeroHash, "0x");
  const receipt = await tx.wait();

  const lockUpdatedEvent = receipt!.logs.find((log) => {
    try {
      const parsed = token.interface.parseLog(log);
      return parsed?.name === "LockCreated";
    } catch {
      return false;
    }
  });

  if (!lockUpdatedEvent) {
    throw new Error("LockUpdated event not found");
  }

  return token.interface.parseLog(lockUpdatedEvent)!.args[0];
}
