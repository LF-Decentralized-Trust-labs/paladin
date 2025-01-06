// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title IZetoPrivate
 * @dev This is the ABI of the Zeto domain transaction interface, which is implemented in Go.
 *      This interface is never expected to be implemented in a smart contract.
 */
interface IZetoPrivate {
    struct TransferParam {
        string to;
        uint256 amount;
    }

    function mint(TransferParam[] memory mints) external;
    function transfer(TransferParam[] memory transfers) external;
    function lock(address delegate, bytes memory call) external;
    function deposit(uint256 amount) external;
    function withdraw(uint256 amount) external;
    function setERC20(address erc20) external;
}
