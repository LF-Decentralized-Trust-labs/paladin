// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "./interfaces/INotoGuard.sol";

contract NotoGuardSimple is INotoGuard {
    function mint(
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external {
        _executeOperation(prepared);
    }

    function transfer(
        address from,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external {
        _executeOperation(prepared);
    }

    function approveTransfer(
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
