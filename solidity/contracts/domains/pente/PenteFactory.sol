// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {PentePrivacyGroup} from "./PentePrivacyGroup.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

contract PenteFactory is IPaladinContractRegistry_V0 {
    PentePrivacyGroup pentePrivacyGroupFactory = new PentePrivacyGroup();

    function newPrivacyGroup(
        bytes32 transactionId,
        bytes memory data
    ) external {
        address instance = Clones.clone(address(pentePrivacyGroupFactory));
        (PentePrivacyGroup(instance)).initialize(transactionId, data);

        emit PaladinRegisterSmartContract_V0(
            transactionId,
            instance,
            data
        );
    }
}
