// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Noto} from "./Noto.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

contract NotoFactory is IPaladinContractRegistry_V0 {
    function deploy(
        bytes32 transactionId,
        address notary,
        bytes memory data
    ) external {
        Noto instance = new Noto(transactionId, notary, data);

        emit PaladinRegisterSmartContract_V0(
            transactionId,
            address(instance),
            data
        );
    }
}
