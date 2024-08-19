// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.24;

import {SIMToken} from "./SIMToken.sol";

// SIMDomain is an un-optimized, simplistic test tool that is used in the unit tests of the test bed
// PLEASE REFER TO ZETO, NOTO AND PENTE FOR REAL EXAMPLES OF ACTUAL IMPLEMENTED DOMAINS
contract SIMDomain {
    
    function newSIMTokenNotarized(
        bytes32 txId,
        address notary,
        string calldata notaryLocator
    ) public {
        // Simply constructs the new contract - which will emit an event
        new SIMToken(txId, address(this), notary, notaryLocator);
    }
    
}
