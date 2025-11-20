// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IConfidentialTokenLockable} from "../interfaces/IConfidentialTokenLockable.sol";

/**
 * @title INoto
 * @dev All implementations of Noto must conform to this interface.
 */
interface INoto is IConfidentialTokenLockable {
    // Options that control how a lock may be utilized.
    // This struct may be ABI-encoded and passed as the "options" parameter to a lock.
    struct LockOptions {
        // A unique transaction ID that must be used to spend or cancel the lock.
        bytes32 spendTxId;

        // Represents a specific spend operation, in the form of an EIP-712 hash over the type:
        //   Unlock(bytes32 txId,bytes32[] lockedInputs,bytes32[] outputs,bytes data)
        // If set to non-zero, this is the only valid outcome for spendLock().
        // A lock may not be delegated unless both spendHash and cancelHash have been prepared.
        bytes32 spendHash;

        // Represents a specific cancel operation, in the form of an EIP-712 hash over the type:
        //   Unlock(bytes32 txId,bytes32[] lockedInputs,bytes32[] outputs,bytes data)
        // If set to non-zero, this is the only valid outcome for cancelLock().
        // A lock may not be delegated unless both spendHash and cancelHash have been prepared.
        bytes32 cancelHash;
    }

    function initialize(
        string memory name_,
        string memory symbol_,
        address notary
    ) external;

    function buildConfig(
        bytes calldata data
    ) external view returns (bytes memory);
}
