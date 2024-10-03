// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "./IPenteExternalCall.sol";

/**
 * A Noto guard is designed to be deployed privately on top of Pente, and to receive
 * prepared transactions from Noto in order to perform final checking and submission
 * to the base ledger.
 */
interface INotoGuard is IPenteExternalCall {
    struct PreparedTransaction {
        address contractAddress;
        bytes encodedCall;
    }

    function mint(
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external;

    function transfer(
        address from,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external;

    function approveTransfer(
        address from,
        address delegate,
        PreparedTransaction calldata prepared
    ) external;
}
