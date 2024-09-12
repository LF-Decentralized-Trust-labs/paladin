// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {ZetoTokenFactory} from "zeto/contracts/factory.sol";
import {IPaladinContractFactory_V0} from "../interfaces/IPaladinContractFactory.sol";

contract ZetoFactory is ZetoTokenFactory, IPaladinContractFactory_V0 {
    function deploy(
        bytes32 transactionId,
        string memory tokenName,
        address initialOwner,
        bytes memory data
    ) external {
        address instance = deployZetoFungibleToken(tokenName, initialOwner);
        emit PaladinNewSmartContractByFactory_V0(
            transactionId,
            instance,
            address(this),
            data
        );
    }
}
