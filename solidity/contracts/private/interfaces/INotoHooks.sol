// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IPenteExternalCall} from "./IPenteExternalCall.sol";

/**
 * @dev Noto hooks can be deployed privately on top of Pente, to receive prepared transactions
 *      from Noto in order to perform final checking and submission to the base ledger.
 */
interface INotoHooks is IPenteExternalCall {
    struct PreparedTransaction {
        address contractAddress;
        bytes encodedCall;
    }

    function onMint(
        address sender,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external;

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external;

    function onBurn(
        address sender,
        address from,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external;

    function onApproveTransfer(
        address sender,
        address from,
        address delegate,
        PreparedTransaction calldata prepared
    ) external;

    function onLock(
        address sender,
        bytes32 id,
        address from,
        uint256 amount,
        address[] calldata recipients,
        PreparedTransaction calldata prepared
    ) external;

    function onUnlock(
        bytes32 id,
        address recipient
    ) external;
}
