// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {Noto} from "./Noto.sol";

contract NotoFactory {
    event PaladinNewSmartContract_V0(
        address indexed contractAddress
    );

    function deploy(address notary) external {
        Noto n = new Noto(notary);
        emit PaladinNewSmartContract_V0(address(n));
    }
}
