// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title INotoPrivate_V0
 * @dev Contains definitions for method signatures unique to the V0 implementation of Noto.
 */
interface INotoPrivate_V0 {
    struct UnlockPublicParams {
        bytes32[] lockedInputs;
        bytes32[] lockedOutputs;
        bytes32[] outputs;
        bytes signature;
        bytes data;
    }

    function delegateLock(
        bytes32 lockId,
        UnlockPublicParams calldata unlock, // removed in V1
        address delegate,
        bytes calldata data
    ) external;
}
