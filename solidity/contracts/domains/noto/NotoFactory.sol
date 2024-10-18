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
        bytes32 notaryType,
        address notaryAddress,
        bytes calldata data
    ) external {
        _deploy(
            implementations["default"],
            transactionId,
            notaryType,
            notaryAddress,
            data
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
     * Query an implementation
     */
    function getImplementation(
        string calldata name    
    ) public view returns (address implementation) {
        return implementations[name];
    }

    /**
     * Deploy an instance of Noto by cloning a specific implementation.
     */
    function deployImplementation(
        string calldata name,
        bytes32 transactionId,
        bytes32 notaryType,
        address notaryAddress,
        bytes calldata data
    ) external {
        _deploy(
            implementations[name],
            transactionId,
            notaryType,
            notaryAddress,
            data
        );
    }

    function _deploy(
        address implementation,
        bytes32 transactionId,
        bytes32 notaryType,
        address notaryAddress,
        bytes calldata data
    ) internal {
        address instance = Clones.clone(implementation);
        bytes memory config = INoto(instance).initialize(
            notaryType,
            notaryAddress,
            data
        );
        emit PaladinRegisterSmartContract_V0(
            transactionId,
            address(instance),
            config
        );
    }
}
