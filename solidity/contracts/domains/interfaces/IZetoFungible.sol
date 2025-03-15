// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title IZetoFungible
 * @dev ABI for the Zeto domain transaction interface, specifically implemented in Go.
 *      Note: This interface is not meant for direct implementation in smart contracts.
 */
interface IZetoFungible {
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