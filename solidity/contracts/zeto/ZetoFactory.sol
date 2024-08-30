// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {ZetoTokenFactory} from "zeto/contracts/factory.sol";
import {IPaladinContract_V0} from "../interfaces/IPaladinContract.sol";

contract ZetoFactory is ZetoTokenFactory, IPaladinContract_V0 {
    function deploy(
        bytes32 transactionId,
        address initialOwner,
        bytes memory data
    ) external {
        address instance = deployZetoFungibleToken("Zeto_Anon", initialOwner);
        emit PaladinNewSmartContractByFactory_V0(
            transactionId,
            instance,
            address(this),
            data
        );
    }
}
