// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

/// @title A simple storage example, enhanced ever so slightly to emit an event when the value changes
contract SimpleStorage {
  uint public storedData;

  event Changed(uint256 x);

  constructor(uint x) {
    set(x);
  }

  function set(uint _x) public {
    storedData = _x;
    emit Changed(storedData);
  }

  function get() view public returns (uint x) {
    return storedData;
  }
}