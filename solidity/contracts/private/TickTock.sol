/**
 * Copyright Kaleido, Inc. 2024.  The materials in this file constitute the "Pre-Existing IP,"
 * "Background IP," "Background Technology" or the like of Kaleido, Inc. and are provided to you
 * under a limited, perpetual license only, subject to the terms of the applicable license
 * agreement between you and Kaleido, Inc.  All other rights reserved.
 */
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.27;

import {TickTockHelper, ITickTock} from "./TickTockHelper.sol";

contract TickTock is ITickTock {
    TickTockHelper public helper;

    event Tick();
    event Tock();
    event Complete();

    constructor(address policy_) {
        helper = TickTockHelper(policy_);
    }

    function tickTock() external {
        helper.triggerTick(address(this));
        emit Tock();
    }

    function tick() external {
        emit Tick();
    }
}
