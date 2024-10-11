// SPDX-License-Identifier: UNLICENSED

pragma solidity ^0.8.13;

contract IdentityRegistry {

    struct Identity {
        bytes32 parent;
        bytes32[] children;
        string name;
        address owner;
    }

    struct Property {
        string name;
        string value;
    }

    event IdentityRegistered (
        bytes32 parentIdentityHash,
        bytes32 identityHash,
        string name,
        address owner
    );

    event PropertySet (
        bytes32 identityHash,
        string name,
        string value
    );

    // Each identity has a unique hash, calculated as a hash of its name and the hash of the parent
    // Identities are stored in a map, from identity hash to identity struct
    // The root identity has key value 0
    mapping(bytes32 => Identity) private identities;

    // Maps identity hashes to property name hashes
    // This is used to list the properties used by each identity
    mapping(bytes32 => bytes32[]) propertyNames;

    // Maps identity hashes to a map of property name hashes to Property structs
    // This is used to store property names and values for each identity
    mapping(bytes32 => mapping(bytes32 => Property)) private properties;

    constructor() {
        // Root identity is created
        Identity memory rootIdentity = Identity(
            0,
            new bytes32[](0),
            "root",
            msg.sender
        );
        identities[0] = rootIdentity;
        emit IdentityRegistered(
            rootIdentity.parent,
            sha256(abi.encodePacked(rootIdentity.parent, rootIdentity.name)),
            rootIdentity.name,
            rootIdentity.owner
        );

        // Root identity is created
        identities[0] = Identity(0, new bytes32[](0), "root", msg.sender);
    }

    function registerIdentity(bytes32 parentIdentityHash, string memory name, address owner) public {
        // Ensure name is not empty
        require(bytes(name).length != 0, "Name cannot be empty");

        // Ensure sender owns parent identity
        require(identities[parentIdentityHash].owner == msg.sender, "Forbidden");

        // Calculate identiy hash based on its name and the hash of the parent identity
        bytes32 hash = sha256(abi.encodePacked(parentIdentityHash, name));

        // Ensure each child has a unique name
        require(bytes(identities[hash].name).length == 0, "Name already taken");

        // Store new identity with a reference to the parent identity, empty list of children, name and owner
        identities[hash] = Identity(parentIdentityHash, new bytes32[](0), name, owner);

        // Store new child identity in parent identity so it can later be listed
        identities[parentIdentityHash].children.push(hash);

        // Emit identity registered event
        emit IdentityRegistered(parentIdentityHash, hash, name, owner);
    }

    function getRootIdentity() public view returns (Identity memory identity) {
        // Returns the root identity which has key 0
        identity = identities[0];
    }

    function getIdentity(bytes32 identityHash) public view returns (Identity memory identity) {
        // Return identity based on hash
        identity = identities[identityHash];

        // Check identity exists
        require(bytes(identity.name).length > 0, "Identity not found");
    }

    function setIdentityProperty(bytes32 identityHash, string memory name, string memory value) public {
        // Ensure name is not empty
        require(bytes(name).length != 0, "Name cannot be empty");

        // Ensure sender owns identity
        require(identities[identityHash].owner == msg.sender, "Forbidden");

        // Calculate property name hash
        bytes32 nameHash = sha256(abi.encodePacked(name));

        // If this is the first time the name is used in the identity, set it up
        if(bytes(properties[identityHash][nameHash].name).length == 0) {
            
            // Store property name
            properties[identityHash][nameHash].name = name;

            // Add propert name hash to identity so it can later be listed
            propertyNames[identityHash].push(nameHash);
        }

        // Store value (or update if already present)
        properties[identityHash][nameHash].value = value;

        // Emit property set value
        emit PropertySet(identityHash, name, value);
    }

    function listIdentityPropertyHashes(bytes32 identityHash) public view returns (bytes32[] memory hashes) {
        // Lists the property name hashes for a given identity
        hashes = propertyNames[identityHash];
    }

    function getIdentityPropertyByHash(bytes32 identityHash, bytes32 propertyNameHash) public view returns(string memory name, string memory value) {
        // Get the property based on the name hash
        Property memory property = properties[identityHash][propertyNameHash];
        
        // Check that the property exists
        require(bytes(property.name).length > 0, "Property not found");

        // Return property name and value
        name = property.name;
        value = property.value;
    }

    function getIdentityPropertyValueByName(bytes32 identityHash, string memory name) public view returns(string memory value) {
        // Calculate name hash
        bytes32 nameHash = sha256(abi.encodePacked(name));

        // Invoke function to return property value
        (, value) = getIdentityPropertyByHash(identityHash, nameHash);
    }

}
