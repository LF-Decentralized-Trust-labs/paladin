// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {ERC1967Proxy} from "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import {INoto} from "../interfaces/INoto.sol";
import {Noto} from "./Noto.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

contract NotoFactory is Ownable, IPaladinContractRegistry_V0 {
    mapping(string => address) internal implementations;

    constructor() Ownable(_msgSender()) {
        implementations["default"] = address(new Noto());
    }

    /**
     * Deploy a default instance of Noto.
     */
    function deploy(
        bytes32 transactionId,
        string calldata notaryLookup,
        bytes32 notaryType,
        address notaryAddress
    ) external {
        _deploy(
            implementations["default"],
            transactionId,
            notaryLookup,
            notaryType,
            notaryAddress
        );
    }

    /**
     * Register an additional implementation of Noto.
     */
    function registerImplementation(
        string calldata name,
        address implementation
    ) public onlyOwner {
        implementations[name] = implementation;
    }

    /**
     * Deploy an instance of Noto by cloning a specific implementation.
     */
    function deployImplementation(
        string calldata name,
        bytes32 transactionId,
        string calldata notaryLookup,
        bytes32 notaryType,
        address notaryAddress
    ) external {
        _deploy(
            implementations[name],
            transactionId,
            notaryLookup,
            notaryType,
            notaryAddress
        );
    }

    function _deploy(
        address implementation,
        bytes32 transactionId,
        string calldata notaryLookup,
        bytes32 notaryType,
        address notaryAddress
    ) internal {
        address instance = Clones.clone(implementation);
        bytes memory data = INoto(instance).initialize(
            notaryLookup,
            notaryType,
            notaryAddress
        );
        emit PaladinRegisterSmartContract_V0(
            transactionId,
            address(instance),
            data
        );
    }
}
