/**
 * Copyright Kaleido, Inc. 2024.  The materials in this file constitute the "Pre-Existing IP,"
 * "Background IP," "Background Technology" or the like of Kaleido, Inc. and are provided to you
 * under a limited, perpetual license only, subject to the terms of the applicable license
 * agreement between you and Kaleido, Inc.  All other rights reserved.
 */
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.27;

import {Address} from "@openzeppelin/contracts/utils/Address.sol";

interface ITickTock {
    function tick() external;
}

contract TickTockHelper {
    using Address for address;

    function triggerTick(address token) external {
        ITickTock(token).tick();
    }
}
