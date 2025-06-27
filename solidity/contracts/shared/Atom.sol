// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Address} from "@openzeppelin/contracts/utils/Address.sol";
import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {ReentrancyGuardUpgradeable} from "@openzeppelin/contracts-upgradeable/utils/ReentrancyGuardUpgradeable.sol";

contract Atom is Initializable, ReentrancyGuardUpgradeable {
    using Address for address;

    enum Status {
        Pending,
        Executed,
        Cancelled
    }

    struct Operation {
        address contractAddress;
        bytes callData;
    }

    struct OperationResult {
        bool success;
        bytes returnData;
    }

    Status public status;

    uint256 private _operationCount;
    Operation[] private _operations;

    event AtomStatusChanged(Status status);

    error AtomNotPending();

    error ExecutionResult(OperationResult[] result);

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    /**
     * Initialize the Atom with a list of operations.
     */
    function initialize(Operation[] memory operations) external initializer {
        __ReentrancyGuard_init();
        status = Status.Pending;
        _operationCount = operations.length;
        for (uint256 i = 0; i < _operationCount; i++) {
            _operations.push(operations[i]);
        }
        emit AtomStatusChanged(status);
    }

    /**
     * Execute the operations in the Atom.
     * Reverts if the Atom has been executed or cancelled, or if any operation fails.
     */
    function execute() external nonReentrant {
        if (status != Status.Pending) {
            revert AtomNotPending();
        }
        status = Status.Executed;

        for (uint256 i = 0; i < _operationCount; i++) {
            Operation storage op = _operations[i];
            op.contractAddress.functionCall(op.callData);
        }
        emit AtomStatusChanged(status);
    }

    /**
     * Simulate the execution of the operations in the Atom.
     * This function always reverts with the encoded results of the operations
     * (even if all operations succeed). It should only be used with eth_call,
     * as a transaction with this method will always revert.
     */
    function simulate()
        external
        nonReentrant
        returns (OperationResult[] memory results)
    {
        if (status != Status.Pending) {
            revert AtomNotPending();
        }

        results = new OperationResult[](_operationCount);
        for (uint256 i = 0; i < _operationCount; i++) {
            Operation storage op = _operations[i];
            (results[i].success, results[i].returnData) = op
                .contractAddress
                .call(op.callData);
        }
        revert ExecutionResult(results);
    }

    /**
     * Cancel the Atom, preventing its execution.
     * Can only be done if the Atom is still pending.
     */
    function cancel() external {
        if (status != Status.Pending) {
            revert AtomNotPending();
        }
        status = Status.Cancelled;
        emit AtomStatusChanged(status);
    }

    function getOperationCount() external view returns (uint256) {
        return _operationCount;
    }

    function getOperation(
        uint256 n
    ) external view returns (Operation memory operation) {
        return _operations[n];
    }

    function getOperations()
        external
        view
        returns (Operation[] memory operations)
    {
        return _operations;
    }
}
