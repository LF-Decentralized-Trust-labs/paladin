// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.24;

interface IPaladinContract_V0 {

    event PaladinPrivateTransaction_V0(
        bytes32   indexed id,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes     data
    );

    function paladinExecute_V0(bytes32 txId, bytes32 fnSelector, bytes calldata payload) external;

}