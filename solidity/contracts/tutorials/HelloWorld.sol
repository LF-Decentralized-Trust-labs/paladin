// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

/**
 * @title HelloWorld
 * @dev A simple contract that emits an event with a welcome message
 */

contract HelloWorld {
    // Define an event
    event HelloEvent(string message);

    /**
     * @dev Emits an event with a welcome message for the given name
     * @param name The name of the person
     */
    function sayHello(string memory name) public {
        // Format the message using string concatenation
        string memory message = string(abi.encodePacked("Welcome to Paladin, ", name, ":)"));
        emit HelloEvent(message); // Emit the event with the formatted message
    }
}
