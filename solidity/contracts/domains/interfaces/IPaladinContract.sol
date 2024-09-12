// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

interface IPaladinContract_V0 {
    event PaladinNewSmartContract_V0(
        bytes32 indexed txId,
        address indexed domain,
        bytes data
    );

    event PaladinPrivateTransaction_V0(
        bytes32 indexed id,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes data
    );

    // TODO: Reconcile if we need this standard function or not - or if domain off-chain code
    //       can simply construct the transactions directly
    // function paladinExecute_V0(bytes32 txId, bytes32 fnSelector, bytes calldata payload) external;
}
