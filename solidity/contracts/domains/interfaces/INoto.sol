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
    event NotoUpdateLock(bytes32 locked, bytes signature, bytes data);
    event NotoUnlock(bytes32 locked, bytes32 outcome, bytes data);

    struct LockOutcome {
        uint64 ref;
        bytes32 state;
    }

    struct TransferParams {
        bytes32[] inputs;
        bytes32[] outputs;
        bytes signature;
    }

    struct LockParams {
        bytes32 locked;
        LockOutcome[] outcomes;
        address delegate;
        bytes signature;
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
        LockOutcome[] calldata outcomes,
        address delegate,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function transferAndLock(
        TransferParams calldata transfer,
        LockParams calldata lock,
        bytes calldata data
    ) external;

    function updateLock(
        bytes32 locked,
        LockOutcome[] calldata outcomes,
        bytes calldata signature,
        bytes calldata data
    ) external;

    function delegateLock(bytes32 locked, address delegate) external;

    function unlock(bytes32 locked, uint64 outcome) external;
}
