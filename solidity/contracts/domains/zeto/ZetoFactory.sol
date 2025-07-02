// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.27;

import {ZetoTokenFactory} from "zeto/contracts/factory.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

contract ZetoFactory is ZetoTokenFactory, IPaladinContractRegistry_V0 {
    function deploy(
        bytes32 transactionId,
        string memory tokenName,
        string memory name,
        string memory symbol,
        address initialOwner,
        bytes memory data,
        bool isNonFungible
    ) external {
        address instance;

        if (isNonFungible) {
            // deploy non-fungible token
            instance = deployZetoNonFungibleToken(
                name,
                symbol,
                tokenName,
                initialOwner
            );
        } else {
            // deploy fungible token
            instance = deployZetoFungibleToken(
                name,
                symbol,
                tokenName,
                initialOwner
            );
        }

        emit PaladinRegisterSmartContract_V0(transactionId, instance, data);
    }

    function deploy(
        bytes32 transactionId,
        string memory tokenName,
        string memory name,
        string memory symbol,
        address initialOwner,
        bytes memory data
    ) external {
        // default deploy is fungible token
        this.deploy(
            transactionId,
            tokenName,
            name,
            symbol,
            initialOwner,
            data,
            false
        );
    }
}
