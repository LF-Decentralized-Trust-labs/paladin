// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {NotoBase} from "./NotoBase.sol";
import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";

/**
 * Noto variant which requires all transfers/approvals to be accompanied by an
 * EIP-712 signature from the notary. Transactions can be submitted by anyone.
 */
contract NotoSigned is NotoBase {
    constructor(address notary) NotoBase(notary) {}

    function transfer(
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes memory signature,
        bytes memory data
    ) external {
        bytes32 txhash = _buildTXHash(inputs, outputs, data);
        address signer = ECDSA.recover(txhash, signature);
        requireNotary(signer);
        _transfer(inputs, outputs, data);
    }

    function approve(
        address delegate,
        bytes32 txhash,
        bytes memory signature
    ) external {
        address signer = ECDSA.recover(txhash, signature);
        requireNotary(signer);
        _approve(delegate, txhash);
    }
}
