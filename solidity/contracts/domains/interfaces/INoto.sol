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

    event NotoLock(
        bytes32 lockId,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes32[] lockedOutputs,
        bytes signature,
        bytes data
    );

    event NotoUnlock(
        bytes32 lockId,
        bytes32[] lockedInputs,
        bytes32[] lockedOutputs,
        bytes32[] outputs,
        bytes signature,
        bytes data
    );

    event NotoUnlockPrepared(
        bytes32 lockId,
        bytes32[] lockedInputs,
        bytes32 unlockHash,
        bytes signature,
        bytes data
    );

    event NotoLockDelegated(
        bytes32 lockId,
        address delegate,
        bytes signature,
        bytes data
    );

    event NotoDelegateUnlock(
        bytes32 lockId,
        address delegate,
        bytes32[] lockedInputs,
        bytes32[] lockedOutputs,
        bytes32[] outputs,
        bytes data
    );

    event NotoUnlockApprovalCancelled(bytes32 lockId, bytes data);

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
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function approveTransfer(
        address delegate,
        bytes32 txhash,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function transferWithApproval(
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function lock(
        bytes32 lockId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes32[] calldata lockedOutputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function unlock(
        bytes32 lockId,
        bytes32[] calldata lockedInputs,
        bytes32[] calldata lockedOutputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function prepareUnlock(
        bytes32 lockId,
        bytes32[] calldata lockedInputs,
        bytes32 unlockHash,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function delegateLock(
        bytes32 lockId,
        address delegate,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function unlockWithApproval(
        bytes32 lockId,
        bytes32[] calldata lockedInputs,
        bytes32[] calldata lockedOutputs,
        bytes32[] calldata outputs,
        bytes calldata data
    ) external;

    function cancelUnlockApproval(bytes32 lockId, bytes calldata data) external;
}
