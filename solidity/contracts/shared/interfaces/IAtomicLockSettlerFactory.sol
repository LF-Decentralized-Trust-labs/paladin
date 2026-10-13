// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IAtomicLockSettler} from "./IAtomicLockSettler.sol";

interface IAtomicLockSettlerFactory {
    function create(IAtomicLockSettler.LockEntry[] calldata locks) external;
}
