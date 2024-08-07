// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

import {SimpleStorage} from './SimpleStorage.sol';

/// @title Embeds a simple storage, and invokes it to do the work
contract SimpleStorageWrapped {

  SimpleStorage ss;

  constructor(uint x) {
    ss = new SimpleStorage(x);
  }

  function set(uint _x) public {
    ss.set(_x);
  }

  function get() view public returns (uint x) {
    return ss.get();
  }
}