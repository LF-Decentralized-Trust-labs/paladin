// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ERC1967Proxy} from "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import {PentePrivacyGroup} from "./PentePrivacyGroup.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

contract PenteFactory is IPaladinContractRegistry_V0 {
    PentePrivacyGroup implementation = new PentePrivacyGroup();

    function newPrivacyGroup(
        bytes32 transactionId,
        bytes memory data
    ) external {
        address instance = address(
            new ERC1967Proxy(
                address(implementation),
                abi.encodeCall(PentePrivacyGroup.initialize, (data))
            )
        );

        emit PaladinRegisterSmartContract_V0(transactionId, instance, data);
    }
}
