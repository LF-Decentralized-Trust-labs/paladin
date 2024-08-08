// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.24;

import {SIMToken} from "./SIMToken.sol";
import {IPaladinDomain_V0} from "../interfaces/IPaladinDomain.sol";

// SIMDomain is an un-optimized, simplistic test tool that is used in the unit tests of the test bed
// PLEASE REFER TO ZETO, NOTO AND PENTE FOR REAL EXAMPLES OF ACTUAL IMPLEMENTED DOMAINS
contract SIMDomain is IPaladinDomain_V0 {
    
    function newSIMTokenNotarized(
        address notary
    ) public {

        SIMToken simToken = new SIMToken(notary);
        
        emit PaladinNewSmartContract_V0(address(simToken));
    }
    
}
