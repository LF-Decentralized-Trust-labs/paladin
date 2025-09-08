// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

// NotoFactory version: 0
interface NotoFactory_V0 is IPaladinContractRegistry_V0 {
    /**
     * Deploy a default instance of Noto.
     */
    function deploy(
        bytes32 transactionId,
        address notary,
        bytes calldata data
    ) external;

    /**
     * Register an additional implementation of Noto.
     */
    function registerImplementation(
        string calldata name,
        address implementation
    ) external;

    /**
     * Query an implementation
     */
    function getImplementation(
        string calldata name
    ) external view returns (address implementation);

    /**
     * Deploy an instance of Noto by cloning a specific implementation.
     */
    function deployImplementation(
        bytes32 transactionId,
        address notary,
        bytes calldata data
    ) external;
}
