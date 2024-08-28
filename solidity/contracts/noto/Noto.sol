// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {NotoBase} from "./NotoBase.sol";

/**
 * Noto variant which requires all transfers/approvals to be submitted by the notary.
 */
contract Noto is NotoBase {
    constructor(address notary) NotoBase(notary) {}

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
