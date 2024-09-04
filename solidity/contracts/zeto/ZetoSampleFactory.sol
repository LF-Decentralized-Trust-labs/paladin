// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Groth16Verifier_CheckHashesValue, Groth16Verifier_CheckInputsOutputsValue, Groth16Verifier_Anon} from "zeto/contracts/zeto_anon.sol";
import {ZetoSample} from "./ZetoSample.sol";

contract ZetoSampleFactory {
    function deploy(
        bytes32 transactionId,
        Groth16Verifier_CheckHashesValue _depositVerifier,
        Groth16Verifier_CheckInputsOutputsValue _withdrawVerifier,
        Groth16Verifier_Anon _verifier,
        bytes memory data
    ) external {
        ZetoSample zeto = new ZetoSample(
            transactionId,
            address(this),
            _depositVerifier,
            _withdrawVerifier,
            _verifier,
            data
        );
        zeto.transferOwnership(msg.sender);
    }
}
