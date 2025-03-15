// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IPenteExternalCall} from "./IPenteExternalCall.sol";

/**
 * @dev Noto hooks can be deployed privately on top of Pente, to receive prepared transactions
 *      from Noto in order to perform final checking and submission to the base ledger.
 *      Unless otherwise noted, each hook should always have one of two outcomes:
 *        - success: the hook should emit "PenteExternalCall" with the prepared transaction in
 *          order to continue submission of the transaction to the base ledger
 *        - failure: the hook should revert with a reason
 */
interface INotoHooks is IPenteExternalCall {
    struct PreparedTransaction {
        address contractAddress;
        bytes encodedCall;
    }

    struct UnlockRecipient {
        address to;
        uint256 amount;
    }

    function onMint(
        address sender,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;

    function onBurn(
        address sender,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;

    function onApproveTransfer(
        address sender,
        address from,
        address delegate,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;

    function onLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;

    function onUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;

    function onPrepareUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;

    function onDelegateLock(
        address sender,
        bytes32 lockId,
        address delegate,
        PreparedTransaction calldata prepared
    ) external;

    /**
     * @dev This method is called after a prepared unlock is executed by the lock delegate.
     *      Unlike other hooks, this method is called after the unlock has been confirmed.
     *      Therefore, this method should never revert, but should only update the state of
     *      the hook contract to reflect the unlock.
     */
    function handleDelegateUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data
    ) external;
}
