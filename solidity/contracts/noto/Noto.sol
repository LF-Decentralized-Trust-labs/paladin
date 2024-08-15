// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {NotoBase} from "./NotoBase.sol";

/**
 * Noto variant which requires all transfers/approvals to be submitted by the notary.
 */
contract Noto is NotoBase {
    event PaladinNewSmartContract_V0(
        bytes32 indexed txId,
        address indexed domain,
        bytes data
    );

    constructor(bytes32 txId, address domain, address notary) NotoBase(notary) {
        emit PaladinNewSmartContract_V0(txId, domain, "");
    }

    function transfer(
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes memory /*signature*/,
        bytes memory data
    ) external onlyNotary {
        _transfer(inputs, outputs, data);
    }

    function approve(
        address delegate,
        bytes32 txhash,
        bytes memory /*signature*/
    ) external onlyNotary {
        _approve(delegate, txhash);
    }
}
