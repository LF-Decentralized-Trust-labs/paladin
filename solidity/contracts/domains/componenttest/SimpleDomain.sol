// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {SimpleToken} from "./SimpleToken.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

// SimpleDomain is an un-optimized, simplistic test tool that is used in the component tests of the core go code
// PLEASE REFER TO ZETO, NOTO AND PENTE FOR REAL EXAMPLES OF ACTUAL IMPLEMENTED DOMAINS
contract SimpleDomain is IPaladinContractRegistry_V0 {
    
    // function parameters here must match the FactoryParameters struct in the go code (simple_domain.go)
    function newSimpleTokenNotarized(
        bytes32 txId,
        string calldata endorsementMode, // "PrivacyGroupEndorsement",  "NotaryEndorsement" or "SelfEndorsement"
        string calldata notaryLocator, // empty string if endorsementMode is not "NotaryEndorsement"
        string[] calldata endorsementSetLocators, // empty array if endorsementMode is not "PrivacyGroupEndorsement"
        string calldata hookAddress // optional hook address for chaining transactions

    ) public {
        // Simply constructs the new contract
        // which will emit an event so that all other paladin nodes can get a copy of the contract config
        SimpleToken instance = new SimpleToken();

        emit PaladinRegisterSmartContract_V0(
            txId,
            address(instance),
            abi.encode(endorsementMode, notaryLocator, endorsementSetLocators, hookAddress)
        );
    }
    
}
