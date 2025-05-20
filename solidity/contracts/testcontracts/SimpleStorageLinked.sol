// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

import {IPenteExternalCall} from "../domains/interfaces/IPenteExternalCall.sol";
import {SimpleStorage} from "./SimpleStorage.sol";

/// @title Tests that PenteExternalCall can propagate all set() calls to another linked contract
contract SimpleStorageLinked is IPenteExternalCall {
    uint public storedData;
    address linkedContract;

    constructor(address linked) {
        linkedContract = linked;
        storedData = 1;
    }

    function set(uint256 value) external {
        storedData = value;
        bytes memory data = abi.encodeCall(SimpleStorage.set, (value));
        emit PenteExternalCall(linkedContract, data); // will be parsed by the Pente domain
    }
}
