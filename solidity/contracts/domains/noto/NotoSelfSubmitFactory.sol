// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {NotoSelfSubmit} from "./NotoSelfSubmit.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

contract NotoSelfSubmitFactory is IPaladinContractRegistry_V0 {
    function deploy(
        bytes32 transactionId,
        address notary,
        bytes memory data
    ) external {
        NotoSelfSubmit instance = new NotoSelfSubmit(notary);

        emit PaladinRegisterSmartContract_V0(
            transactionId,
            address(instance),
            data
        );
    }
}
