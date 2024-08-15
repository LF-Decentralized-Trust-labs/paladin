// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {Noto} from "./Noto.sol";

contract NotoFactory {
    function deploy(bytes32 txId, address notary) external {
        new Noto(txId, address(this), notary);
    }
}
