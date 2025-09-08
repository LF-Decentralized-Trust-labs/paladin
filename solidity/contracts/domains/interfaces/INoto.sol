// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title INoto
 * @dev All implementations of Noto must conform to this interface.
 */
interface INoto {
    event NotoTransfer(
        bytes32 txId,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes signature,
        bytes data
    );

    event NotoLock(
        bytes32 txId,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes32[] lockedOutputs,
        bytes signature,
        bytes data
    );

    event NotoUnlock(
        bytes32 txId,
        address sender,
        bytes32[] lockedInputs,
        bytes32[] lockedOutputs,
        bytes32[] outputs,
        bytes signature,
        bytes data
    );

    event NotoUnlockPrepared(
        bytes32[] lockedInputs,
        bytes32 unlockHash,
        bytes signature,
        bytes data
    );

    event NotoLockDelegated(
        bytes32 txId,
        bytes32 unlockHash,
        address delegate,
        bytes signature,
        bytes data
    );

    function initialize(
        string memory name_,
        string memory symbol_,
        address notary
    ) external;

    function buildConfig(
        bytes calldata data
    ) external view returns (bytes memory);

    function mint(
        bytes32 txId,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function transfer(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function lock(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes32[] calldata lockedOutputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function unlock(
        bytes32 txId,
        bytes32[] calldata lockedInputs,
        bytes32[] calldata lockedOutputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function prepareUnlock(
        bytes32[] calldata lockedInputs,
        bytes32 unlockHash,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function delegateLock(
        bytes32 txId,
        bytes32 unlockHash,
        address delegate,
        bytes calldata signature,
        bytes calldata data
    ) external;
}
