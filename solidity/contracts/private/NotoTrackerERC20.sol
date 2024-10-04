// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "./interfaces/INotoGuard.sol";

/**
 * Example Noto Guard which tracks all Noto token movements on a private ERC20.
 */
contract NotoTrackerERC20 is INotoGuard, ERC20 {
    constructor(string memory name, string memory symbol) ERC20(name, symbol) {}

    function onMint(
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external {
        _mint(to, amount);
        _executeOperation(prepared);
    }

    function onTransfer(
        address from,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external {
        _transfer(from, to, amount);
        _executeOperation(prepared);
    }

    function onApproveTransfer(
        address from,
        address delegate,
        PreparedTransaction calldata prepared
    ) external {
        _executeOperation(prepared);
    }

    function _executeOperation(PreparedTransaction memory op) internal {
        // TODO: replace with Pente event
        // emit PenteExternalCall(op.contractAddress, op.encodedCall);
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
