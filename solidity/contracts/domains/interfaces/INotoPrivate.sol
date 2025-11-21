// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title INotoPrivate
 * @dev This is the ABI of the Noto private transaction interface, which is implemented in Go.
 *      This interface is never expected to be implemented in a smart contract.
 */
interface INotoPrivate {
    struct UnlockRecipient {
        string to;
        uint256 amount;
    }

    function mint(
        string calldata to,
        uint256 amount,
        bytes calldata data
    ) external;

    function burn(uint256 amount, bytes calldata data) external;

    function burnFrom(
        string calldata from,
        uint256 amount,
        bytes calldata data
    ) external;

    function transfer(
        string calldata to,
        uint256 amount,
        bytes calldata data
    ) external;

    function transferFrom(
        string calldata from,
        string calldata to,
        uint256 amount,
        bytes calldata data
    ) external;

    // @deprecated - use createLock instead
    function lock(uint256 amount, bytes calldata data) external;

    function createLock(uint256 amount, bytes calldata data) external;

    function createMintLock(
        UnlockRecipient[] calldata recipients,
        bytes calldata data
    ) external;

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

    function prepareBurnUnlock(
        bytes32 lockId,
        string calldata from,
        uint256 amount,
        bytes calldata data
    ) external;

    function delegateLock(
        bytes32 lockId,
        address delegate,
        bytes calldata data
    ) external;

    function name() external view returns (string memory);

    function symbol() external view returns (string memory);

    function decimals() external view returns (uint8);

    function balanceOf(
        string memory account
    )
        external
        view
        returns (uint256 totalStates, uint256 totalBalance, bool overflow);
}
