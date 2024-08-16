// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.24;

import {IPaladinContract_V0} from "../interfaces/IPaladinContract.sol";

// SIMDomain is an un-optimized, simplistic test tool that is used in the unit tests of the test bed
// PLEASE REFER TO ZETO, NOTO AND PENTE FOR REAL EXAMPLES OF ACTUAL IMPLEMENTED DOMAINS
contract SIMToken is IPaladinContract_V0 {

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
    
    constructor(bytes32 txId, address domain, address notary, string memory notaryLocator) {
        _notary = notary;

        emit PaladinNewSmartContract_V0(txId, domain, abi.encode(notaryLocator));
    }

    function paladinExecute_V0(bytes32 txId, bytes32 fnSelector, bytes calldata payload) public onlyNotary {
        assert(fnSelector == SINGLE_FUNCTION_SELECTOR);
        (bytes32 signature, bytes32[] memory inputs, bytes32[] memory outputs) =
            abi.decode(payload, (bytes32, bytes32[], bytes32[]));
        emit PaladinPrivateTransaction_V0(txId, inputs, outputs, abi.encodePacked(signature));
    }

    function executeNotarized(bytes32 txId, bytes32[] calldata inputs, bytes32[] calldata outputs, bytes calldata signature) public onlyNotary {
        emit PaladinPrivateTransaction_V0(txId, inputs, outputs, abi.encodePacked(signature));
    }
}
