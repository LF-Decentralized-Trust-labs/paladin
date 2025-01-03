// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

/**
 * @title HelloWorld
 * @dev A simple contract that returns a string "Hello, world!"
 */

contract HelloWorld {
    function helloWorld() public pure returns (string memory) {
        return "Hello, world!";
    }
}
