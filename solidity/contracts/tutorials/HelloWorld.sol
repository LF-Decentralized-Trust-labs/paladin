// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

/**
 * @title HelloWorld
 * @dev A simple contract that emits an event with a welcome message
 */
contract HelloWorld {
    /// @notice Emitted when a user is greeted
    /// @param message The generated greeting message
    event HelloEvent(string message);

    /**
     * @dev Emits a personalized welcome message for the given name
     * @param name The name of the recipient
     */
    function sayHello(string memory name) public {
        string memory message = string.concat("Welcome to Paladin, ", name);

        // Emit the event with the formatted message        
        emit HelloEvent(message);
    }
}
