// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Address} from "@openzeppelin/contracts/utils/Address.sol";
import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";

contract Atom is Initializable {
    using Address for address;

    uint256 private _operationCount;
    Operation[] private _operations;
    bool public cancelled;

    struct Operation {
        address contractAddress;
        bytes callData;
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(Operation[] memory operations) public initializer {
        _operationCount = operations.length;
        for (uint256 i = 0; i < _operationCount; i++) {
            _operations.push(operations[i]);
        }
    }

    function execute() public {
        require(!cancelled, "Atom has been cancelled");
        for (uint256 i = 0; i < _operationCount; i++) {
            _executeOperation(_operations[i]);
        }
    }

    function cancel() public {
        require(!cancelled, "Atom has already been cancelled");
        cancelled = true;
    }

    function _executeOperation(Operation storage op) internal {
        op.contractAddress.functionCall(op.callData);
    }

    function getOperationCount() public view returns (uint256) {
        return _operationCount;
    }

    function getOperation(uint256 n) public view returns (Operation memory) {
        return _operations[n];
    }
}

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
