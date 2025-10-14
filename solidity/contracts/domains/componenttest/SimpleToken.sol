// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

// SIMDomain is an un-optimized, simplistic test tool that is used in the unit tests of the test bed
// PLEASE REFER TO ZETO, NOTO AND PENTE FOR REAL EXAMPLES OF ACTUAL IMPLEMENTED DOMAINS
contract SimpleToken {

    event UTXOTransfer(
        bytes32 txId,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes signature
    );

    error BadNotary(address sender);
    bytes32 public constant SINGLE_FUNCTION_SELECTOR = keccak256("SimpleToken()");
    
    constructor() {
    }

    function paladinExecute_V0(bytes32 txId, bytes32 fnSelector, bytes calldata payload) public  {
        assert(fnSelector == SINGLE_FUNCTION_SELECTOR);
        (bytes32 signature, bytes32[] memory inputs, bytes32[] memory outputs) =
            abi.decode(payload, (bytes32, bytes32[], bytes32[]));
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
    }

    function executeNotarized(bytes32 txId, bytes32[] calldata inputs, bytes32[] calldata outputs, bytes calldata signature) public {

        // Special value which the simple domain can pass to cause a revert, by fixing to a known salt & owner
        if (outputs.length > 0 && outputs[0] == hex"1e7d508a2dd3e97c760b48f999658e0a55f47e825e00e1599725d366ccf31d01") {
            revert("simple domain revert");
        }
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
    }

    function executeNotarizedHook(bytes32 txId, bytes32[] calldata inputs, bytes32[] calldata outputs, bytes calldata signature, bytes32 originTxId) public {
        // Emit 2 events, one for the hook TX ID, one for the original TX ID. Note that the simple domain
        // doesn't check the inputs and outputs so we just pass them through to both. In reality the origin
        // domain wouldn't validate the inputs and outputs but we're just testing TX chaining here, not domain functionality.
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
        emit UTXOTransfer(originTxId, inputs, outputs, abi.encodePacked(signature));
    }

}
