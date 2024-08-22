// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {Zeto_Anon, Groth16Verifier_CheckHashesValue, Groth16Verifier_CheckInputsOutputsValue, Groth16Verifier_Anon} from "zeto/contracts/zeto_anon.sol";
import {IPaladinContract_V0} from "../interfaces/IPaladinContract.sol";

contract ZetoSample is Zeto_Anon, IPaladinContract_V0 {
    constructor(
        bytes32 transactionId,
        address domain,
        Groth16Verifier_CheckHashesValue _depositVerifier,
        Groth16Verifier_CheckInputsOutputsValue _withdrawVerifier,
        Groth16Verifier_Anon _verifier,
        bytes memory data
    ) Zeto_Anon(_depositVerifier, _withdrawVerifier, _verifier) {
        emit PaladinNewSmartContract_V0(transactionId, domain, data);
    }
}
