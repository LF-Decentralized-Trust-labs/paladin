// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/// A contract registry is a trusted smart contract that announces the existence of
/// new smart contracts instances of that are part of a single domain.
/// @author https://github.com/kaleido-io/paladin
/// @title Paladin contract registry
interface IPaladinContractRegistry_V0 {

    /// The event emitted when a new smart contract is deployed, provides the public
    /// configuration information that is needed to configure the off-chain components
    /// of the domain to correctly function against the new instance.
    /// @param txId the submitter-determined transaction id is used to correlate the event to the submitting business transaction
    /// @param instance the address of the instance smart contract that has been deployed
    /// @param config encoded parameters that all nodes functioning against this smart contact instance need to know
    event PaladinRegisterSmartContract_V0(
        bytes32 indexed txId,
        address indexed instance,
        bytes config
    );

}
