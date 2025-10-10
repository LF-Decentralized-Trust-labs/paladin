// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {Atom} from "./Atom.sol";

contract AtomFactory {
    address public immutable logic;

    event AtomDeployed(address addr);

    constructor() {
        logic = address(new Atom());
    }

    function create(Atom.Operation[] calldata operations) public {
        address instance = Clones.clone(logic);
        Atom(instance).initialize(operations);
        emit AtomDeployed(instance);
    }
}
