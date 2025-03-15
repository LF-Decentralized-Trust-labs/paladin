// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/**
 * @title Storage
 * @dev Store & retrieve a value in a variable
 */
contract Storage {
    uint256 private _number;

    /**
     * @dev Store a value in the contract
     * @param num The value to store
     */
    function store(uint256 num) public {
        _number = num;
    }

    /**
     * @dev Retrieve the stored value
     * @return value The currently stored value
     */
    function retrieve() public view returns (uint256 value) {
        return _number;
    }
}
