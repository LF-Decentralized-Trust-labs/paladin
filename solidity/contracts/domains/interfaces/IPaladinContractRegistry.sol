// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

// This is kept separate from the IPaladinContract_V0 interface to allow
// for separate event streams attached to specific trusted factory addresses.
// The IPaladinContract_V0 events can be indexed from the whole ledger
// because the contract address is parsed from the protocol-generated data
// which can be trusted to be correct.
interface IPaladinContractRegistry_V0 {
    event PaladinRegisterSmartContract_V0(
        bytes32 indexed txId,
        address indexed instance,
        bytes data
    );
}
