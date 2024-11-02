// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {INotoHooks} from "../private/interfaces/INotoHooks.sol";

/**
 * Example Noto hooks which track all Noto token movements on a public ERC20.
 * This version is useful only as a proof-of-concept for testing, as mirroring all token
 * movements to a public contract defeats the purpose of using Noto to begin with.
 * TODO: remove when all functionality is tested using Pente instead of base ledger.
 */
contract NotoTrackerPublicERC20 is INotoHooks, ERC20 {
    constructor(string memory name, string memory symbol) ERC20(name, symbol) {}

    function onMint(
        address sender,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external {
        _mint(to, amount);
        _executeOperation(prepared);
    }

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external {
        _transfer(from, to, amount);
        _executeOperation(prepared);
    }

    function onApproveTransfer(
        address sender,
        address from,
        address delegate,
        PreparedTransaction calldata prepared
    ) external {
        _executeOperation(prepared);
    }

    function _executeOperation(PreparedTransaction memory op) internal {
        (bool success, bytes memory result) = op.contractAddress.call(
            op.encodedCall
        );
        if (!success) {
            assembly {
                // Forward the revert reason
                let size := mload(result)
                let ptr := add(result, 32)
                revert(ptr, size)
            }
        }
    }
}
