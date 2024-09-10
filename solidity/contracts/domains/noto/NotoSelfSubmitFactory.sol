// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ERC1967Proxy} from "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import {NotoSelfSubmit} from "./NotoSelfSubmit.sol";

contract NotoSelfSubmitFactory {
    address public immutable logic;

    string private constant INIT_SIGNATURE =
        "initialize(bytes32,address,address,bytes)";

    constructor() {
        logic = address(new NotoSelfSubmit());
    }

    function deploy(
        bytes32 transactionId,
        address notary,
        bytes memory data
    ) external {
        bytes memory _initializationCalldata = abi.encodeWithSignature(
            INIT_SIGNATURE,
            transactionId,
            address(this),
            notary,
            data
        );
        new ERC1967Proxy(logic, _initializationCalldata);
    }
}
