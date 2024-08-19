// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {NotoBase} from "./NotoBase.sol";
import {IPaladinContract_V0} from "../interfaces/IPaladinContract.sol";

/**
 * Noto variant which requires all transfers/approvals to be submitted by the notary.
 */
contract Noto is NotoBase {
    constructor(bytes32 txId, address domain, address notary) NotoBase(notary) {
        emit IPaladinContract_V0.PaladinNewSmartContract_V0(txId, domain, abi.encode(notary));
    }

    function transfer(
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes memory /*signature*/,
        bytes memory data
    ) external onlyNotary {
        _transfer(inputs, outputs, data);
        // bytes32 id = abi.decode(data, (bytes32));
        // emit IPaladinContract_V0.PaladinPrivateTransaction_V0(id, inputs, outputs, "");
    }

    function approve(
        address delegate,
        bytes32 txhash,
        bytes memory /*signature*/
    ) external onlyNotary {
        _approve(delegate, txhash);
    }
}
