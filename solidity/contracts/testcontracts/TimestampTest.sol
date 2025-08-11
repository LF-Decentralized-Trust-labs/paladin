// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

/// @title Test that block.timestamp is in a sane range
contract TimestampTest {
  uint256 public MIN = 946702800; // 2000-01-01
  uint256 public MAX = 4102462800; // 2100-01-01

  constructor() {
    require(block.timestamp > MIN, "block.timestamp is before 2000-01-01");
    require(block.timestamp < MAX, "block.timestamp is after 2100-01-01");
  }
}
