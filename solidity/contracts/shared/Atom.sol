// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC721/IERC721.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import "../domains/interfaces/INoto.sol";

contract Atom is Initializable {
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
        (bool success, bytes memory result) = op.contractAddress.call(
            op.callData
        );
        if (!success) {
            assembly {
                // Forward the revert reason
                let size := mload(result)
                let ptr := add(result, 32)
                revert(ptr, size)
            }
        }
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

    // Must match the signature initialize(Atom.Operation[])
    string private constant INIT_SIGNATURE = "initialize((address,bytes)[])";

    constructor() {
        logic = address(new Atom());
    }

    function create(Atom.Operation[] calldata operations) public {
        bytes memory _initializationCalldata = abi.encodeWithSignature(
            INIT_SIGNATURE,
            operations
        );
        address addr = address(
            new ERC1967Proxy(logic, _initializationCalldata)
        );
        emit AtomDeployed(addr);
    }
}
