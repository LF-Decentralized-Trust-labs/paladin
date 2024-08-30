// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {PentePrivacyGroup} from "./PentePrivacyGroup.sol";
import {IPente} from "../interfaces/IPente.sol";

/// @title 
/// @author Kaleido, Inc.
/// @dev 
///
contract PenteSmartContract is IPente, IPaladinContract_V0 {
    PentePrivacyGroup _privacyGroup;
    bytes32 _state;

    error PenteNotPrivacyGroup(address sender);
    error PenteBadRoot(bytes32 provided, bytes32 expected);

    function requirePrivacyGroup(address addr) internal view {
        if (addr != _privacyGroup) {
            revert PenteNotPrivacyGroup(addr);
        }
    }

    modifier onlyPrivacyGroup() {
        requirePrivacyGroup(msg.sender);
        _;
    }

    constructor(bytes32 transactionId, address privacyGroup, bytes memory data) {
        _privacyGroup = privacyGroup;
    }

    /**
     * @dev transitions the state this account / smart contract within the privacy group
     *
     * @param oldState previous state hash
     * @param newState new state hash
     */
    function transition(
        bytes32 oldState,
        bytes32 newState
    ) public onlyPrivacyGroup {
        if (_state != old) {
            revert PenteBadRoot(old, _state);
        }
    }
}
