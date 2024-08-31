// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {PentePrivacyGroup} from "./PentePrivacyGroup.sol";

contract PenteFactory {
    PentePrivacyGroup pentePrivacyGroupFactory = new PentePrivacyGroup();

    function deploy(
        bytes32 transactionId,
        bytes memory config
    ) external {
        address instance = Clones.clone(address(pentePrivacyGroupFactory));
        (PentePrivacyGroup(instance)).initialize(transactionId, address(this), config);
    }
}
