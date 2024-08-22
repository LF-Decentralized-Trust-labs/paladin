// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {Noto} from "./Noto.sol";

contract NotoFactory {
    function deploy(
        bytes32 transactionId,
        address notary,
        bytes memory data
    ) external {
        new Noto(transactionId, address(this), notary, data);
    }
}
