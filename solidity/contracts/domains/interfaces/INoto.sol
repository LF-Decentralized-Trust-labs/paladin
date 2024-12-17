// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title INoto
 * @dev All implementations of Noto must conform to this interface.
 */
interface INoto {
    event NotoTransfer(
        bytes32[] inputs,
        bytes32[] outputs,
        bytes signature,
        bytes data
    );

    event NotoApproved(
        address delegate,
        bytes32 txhash,
        bytes signature,
        bytes data
    );

    event NotoLock(bytes32 locked, bytes signature, bytes data);

    event NotoUnlock(bytes32 locked, bytes32 output, bytes data);

    struct LockInput {
        bytes32 revertOutput;
        address delegate;
    }

    function initialize(
        address notaryAddress,
        bytes calldata data
    ) external returns (bytes memory);

    function mint(
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function transfer(
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes memory signature,
        bytes memory data
    ) external;

    function approveTransfer(
        address delegate,
        bytes32 txhash,
        bytes memory signature,
        bytes memory data
    ) external;

    function transferWithApproval(
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes memory signature,
        bytes memory data
    ) external;

    function createLock(
        bytes32 locked,
        LockInput calldata lock,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function addLockOutcome(
        bytes32 locked,
        uint64 ref,
        bytes32 outcome
    ) external;

    function removeLockOutcome(bytes32 locked, uint64 ref) external;

    function delegateLock(bytes32 locked, address delegate) external;

    function unlock(bytes32 locked, uint64 outcome) external;

    function transferAndLock(
        bytes32[] calldata inputs,
        bytes32[] calldata unlockedOutputs,
        bytes32 lockedOutput,
        LockInput calldata lock,
        bytes calldata signature,
        bytes calldata data
    ) external;
}
