// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC721/IERC721.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import "../interfaces/INoto.sol";

contract MultiCall is Initializable {
    uint256 private _operationCount;
    Operation[] private _operations;

    enum OperationType {
        EncodedCall
    }

    struct Operation {
        OperationType opType;
        address contractAddress;
        bytes data;
    }

    error MultiCallUnsupportedType(OperationType opType);

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(Operation[] memory operations) public initializer {
        _operationCount = operations.length;
        for (uint256 i = 0; i < _operationCount; i++) {
            if (operations[i].opType == OperationType.EncodedCall) {
                _operations.push(
                    Operation(
                        operations[i].opType,
                        operations[i].contractAddress,
                        operations[i].data
                    )
                );
            } else {
                revert MultiCallUnsupportedType(operations[i].opType);
            }
        }
    }

    function execute() public {
        for (uint256 i = 0; i < _operationCount; i++) {
            _executeOperation(_operations[i]);
        }
    }

    function _executeOperation(Operation storage op) internal {
        if (op.opType == OperationType.EncodedCall) {
            (bool success, bytes memory result) = op.contractAddress.call(
                op.data
            );
            if (!success) {
                assembly {
                    // Forward the revert reason
                    let size := mload(result)
                    let ptr := add(result, 32)
                    revert(ptr, size)
                }
            }
        } else {
            revert MultiCallUnsupportedType(op.opType);
        }
    }

    function getOperationCount() public view returns (uint256) {
        return _operationCount;
    }
}

contract MultiCallFactory {
    address public immutable logic;

    event MultiCallDeployed(address addr);

    // Must match the signature initialize(MultiCall.Operation[])
    string private constant INIT_SIGNATURE =
        "initialize((uint8,address,bytes)[])";

    constructor() {
        logic = address(new MultiCall());
    }

    function create(MultiCall.Operation[] calldata operations) public {
        bytes memory _initializationCalldata = abi.encodeWithSignature(
            INIT_SIGNATURE,
            operations
        );
        address addr = address(
            new ERC1967Proxy(logic, _initializationCalldata)
        );
        emit MultiCallDeployed(addr);
    }
}
