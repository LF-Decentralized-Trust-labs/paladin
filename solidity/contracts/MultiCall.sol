// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC721/IERC721.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import "zeto/contracts/lib/common.sol";
import "zeto/contracts/zeto_anon.sol";
import "./interfaces/INoto.sol";

contract MultiCall is Initializable {
    uint256 private _operationCount;
    Operation[] private _operations;

    enum OperationType {
        EncodedCall,
        ERC20Transfer,
        ERC721Transfer,
        NotoTransfer,
        ZetoTransfer
    }

    struct OperationInput {
        OperationType opType;
        address contractAddress;
        address fromAddress;
        address toAddress;
        uint256 value;
        uint256 tokenIndex;
        uint256[] inputs;
        uint256[] outputs;
        bytes signature;
        bytes data;
        Commonlib.Proof proof;
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

    function initialize(OperationInput[] memory operations) public initializer {
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
            } else if (operations[i].opType == OperationType.ERC20Transfer) {
                _operations.push(
                    Operation(
                        operations[i].opType,
                        operations[i].contractAddress,
                        abi.encode(
                            operations[i].fromAddress,
                            operations[i].toAddress,
                            operations[i].value
                        )
                    )
                );
            } else if (operations[i].opType == OperationType.ERC721Transfer) {
                _operations.push(
                    Operation(
                        operations[i].opType,
                        operations[i].contractAddress,
                        abi.encode(
                            operations[i].fromAddress,
                            operations[i].toAddress,
                            operations[i].tokenIndex,
                            operations[i].data
                        )
                    )
                );
            } else if (operations[i].opType == OperationType.NotoTransfer) {
                _operations.push(
                    Operation(
                        operations[i].opType,
                        operations[i].contractAddress,
                        abi.encode(
                            operations[i].inputs,
                            operations[i].outputs,
                            operations[i].data
                        )
                    )
                );
            } else if (operations[i].opType == OperationType.ZetoTransfer) {
                _operations.push(
                    Operation(
                        operations[i].opType,
                        operations[i].contractAddress,
                        abi.encode(
                            operations[i].inputs,
                            operations[i].outputs,
                            operations[i].proof
                        )
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
        } else if (op.opType == OperationType.ERC20Transfer) {
            (address fromAddress, address toAddress, uint256 value) = abi
                .decode(op.data, (address, address, uint256));
            IERC20(op.contractAddress).transferFrom(
                fromAddress,
                toAddress,
                value
            );
        } else if (op.opType == OperationType.ERC721Transfer) {
            (
                address fromAddress,
                address toAddress,
                uint256 tokenIndex,
                bytes memory data
            ) = abi.decode(op.data, (address, address, uint256, bytes));
            IERC721(op.contractAddress).safeTransferFrom(
                fromAddress,
                toAddress,
                tokenIndex,
                data
            );
        } else if (op.opType == OperationType.NotoTransfer) {
            (
                bytes32[] memory inputs,
                bytes32[] memory outputs,
                bytes memory data
            ) = abi.decode(op.data, (bytes32[], bytes32[], bytes));
            INoto(op.contractAddress).approvedTransfer(inputs, outputs, data);
        } else if (op.opType == OperationType.ZetoTransfer) {
            (
                uint256[] memory inputs,
                uint256[] memory outputs,
                Commonlib.Proof memory proof
            ) = abi.decode(op.data, (uint256[], uint256[], Commonlib.Proof));
            require(inputs.length == 2);
            require(outputs.length == 2);
            Zeto_Anon(op.contractAddress).transfer(
                [inputs[0], inputs[1]],
                [outputs[0], outputs[1]],
                proof
            );
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

    // Must match the signature initialize(MultiCall.OperationInput[])
    string private constant INIT_SIGNATURE =
        "initialize((uint8,address,address,address,uint256,uint256,uint256[],uint256[],bytes,bytes,(uint256[2],uint256[2][2],uint256[2]))[])";

    constructor() {
        logic = address(new MultiCall());
    }

    function create(MultiCall.OperationInput[] calldata operations) public {
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
