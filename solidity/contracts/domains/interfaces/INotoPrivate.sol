// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title INotoPrivate
 * @dev This is the ABI of the Noto private transaction interface, which is implemented in Go.
 *      This interface is never expected to be implemented in a smart contract.
 */
interface INotoPrivate {
    function mint(
        string calldata to,
        uint256 amount,
        bytes calldata data
    ) external;

    function transfer(
        string calldata to,
        uint256 amount,
        bytes calldata data
    ) external;

    function burn(uint256 amount, bytes calldata data) external;

    function approveTransfer(
        StateEncoded[] calldata inputs,
        StateEncoded[] calldata outputs,
        bytes calldata data,
        address delegate
    ) external;

    function lock(uint256 amount, bytes calldata data) external;

    function unlock(
        bytes32 lockId,
        string calldata from,
        UnlockRecipient[] calldata recipients,
        bytes calldata data
    ) external;

    function prepareUnlock(
        bytes32 lockId,
        string calldata from,
        UnlockRecipient[] calldata recipients,
        bytes calldata data
    ) external;

    function delegateLock(
        bytes32 lockId,
        UnlockPublicParams calldata unlock,
        address delegate,
        bytes calldata data
    ) external;

    function balanceOf(string memory account) external view returns (uint256 totalStates, uint256 totalBalance, bool overflow);

    struct StateEncoded {
        bytes id;
        string domain;
        bytes32 schema;
        address contractAddress;
        bytes data;
    }

    struct UnlockRecipient {
        string to;
        uint256 amount;
    }

    struct UnlockPublicParams {
        bytes32[] lockedInputs;
        bytes32[] lockedOutputs;
        bytes32[] outputs;
        bytes signature;
        bytes data;
    }
}
