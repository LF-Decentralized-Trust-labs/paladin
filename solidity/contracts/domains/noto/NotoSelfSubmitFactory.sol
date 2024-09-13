// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {NotoSelfSubmit} from "./NotoSelfSubmit.sol";

contract NotoSelfSubmitFactory {
    function deploy(
        bytes32 transactionId,
        address notary,
        bytes memory data
    ) external {
        new NotoSelfSubmit(transactionId, notary, data);
    }
}
