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
        // Represents a specific unlock operation, in the form of an EIP-712 hash over the type:
        //   Unlock(bytes32[] lockedInputs,bytes32[] lockedOutputs,bytes32[] outputs,bytes data)
        // If set to non-zero, this is the only valid outcome for the lock.
        bytes32 unlockHash;

        // The time at which a lock delegation expires, in seconds since the Unix epoch.
        // If set to zero, the lock delegation never expires.
        // After expiration, any lock delegation is ignored, and control of the locked
        // states returns to the original creator.
        uint256 expiration;
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
