// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

// SIMDomain is an un-optimized, simplistic test tool that is used in the unit tests of the test bed
// PLEASE REFER TO ZETO, NOTO AND PENTE FOR REAL EXAMPLES OF ACTUAL IMPLEMENTED DOMAINS
contract SIMToken {

    event UTXOTransfer(
        bytes32 txId,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes signature
    );

    address _notary;

    error BadNotary(address sender);

    // SIMToken doesn't have multiple functions that require different logic,
    // but demonstrates validating the function selector equals a value
    bytes32 public constant SINGLE_FUNCTION_SELECTOR = keccak256("SIMToken()");
    
    function requireNotary(address addr) internal view {
        if (addr != _notary) {
            revert BadNotary(addr);
        }
    }

    modifier onlyNotary() {
        requireNotary(msg.sender);
        _;
    }
    
    constructor(address notary) {
        _notary = notary;
    }

    function paladinExecute_V0(bytes32 txId, bytes32 fnSelector, bytes calldata payload) public onlyNotary {
        assert(fnSelector == SINGLE_FUNCTION_SELECTOR);
        (bytes32 signature, bytes32[] memory inputs, bytes32[] memory outputs) =
            abi.decode(payload, (bytes32, bytes32[], bytes32[]));
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
    }

    function executeNotarized(bytes32 txId, bytes32[] calldata inputs, bytes32[] calldata outputs, bytes calldata signature) public onlyNotary {
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
    }
}
