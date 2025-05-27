// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title IZetoNonFungible
 * @dev ABI for the Zeto domain transaction interface, specifically implemented in Go.
 *      Note: This interface is not intended for direct implementation in smart contracts.
 */
interface IZetoNonFungible {
    struct MintParam {
        string to;
        string uri;
    }
    
    struct TransferParam {
        string to;
        uint256 tokenID;
    }

    function mint(MintParam[] memory mints) external;
    function transfer(TransferParam[] memory transfers) external;
}