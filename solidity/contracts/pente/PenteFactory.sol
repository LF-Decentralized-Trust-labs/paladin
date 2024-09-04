// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {PentePrivacyGroup} from "./PentePrivacyGroup.sol";

contract PenteFactory {
    PentePrivacyGroup pentePrivacyGroupFactory = new PentePrivacyGroup();

    function newPrivacyGroup(
        bytes32 transactionId,
        bytes memory config
    ) external {
        address instance = Clones.clone(address(pentePrivacyGroupFactory));
        (PentePrivacyGroup(instance)).initialize(transactionId, address(this), config);
    }
}
