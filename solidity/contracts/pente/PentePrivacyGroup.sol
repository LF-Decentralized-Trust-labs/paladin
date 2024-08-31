// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {EIP712Upgradeable} from "@openzeppelin/contracts-upgradeable/utils/cryptography/EIP712Upgradeable.sol";
import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {IPaladinContract_V0} from "../interfaces/IPaladinContract.sol";

/// @title Base smart contract for pinning privacy group endorsed private smart
///      contract state transitions to the base ledger.
/// @author Kaleido, Inc.
/// @dev Each transition is a UTXO transaction:
///      - Spending the previous account state of the privacy group (private nonces)
///      - Spending the previous account state of smart contracts in this privacy group
///      - TODO: Spending privacy group / account states on accounts in other other privacy groups atomically
///
contract PentePrivacyGroup is UUPSUpgradeable, EIP712Upgradeable, IPaladinContract_V0 {
    bytes32 private constant TRANSITION_TYPEHASH =
        keccak256("Transitions(bytes32[] accounts,bytes32[] oldStates,bytes32[] newStates)");
    bytes32 private constant UPGRADE_TYPEHASH =
        keccak256("Upgrade(address address)");

    mapping(bytes32 => bytes32) _accountStates; // accounts are referenced by a salted hash
    address _nextImplementation;

    // Config follows the convention of a 4 byte type selector, followed by ABI encoded bytes
    bytes4 public constant PenteConfigID_Endorsement_V0 = 0x00010000;

    error PenteUnsupportedConfigType(bytes4 configSelector);
    error PenteDuplicateEndorser(address signer);
    error PenteInvalidEndorser(address signer);
    error PenteEndorsementThreshold(uint obtained, uint required);
    error PenteUpgradeNotPreauthorized();
    error PenteAccountStateMismatch(bytes32 asserted, bytes32 actual);

    struct EndorsementConfig_V0 {
        uint      threshold;
        address[] endorsmentSet;
    }

    EndorsementConfig_V0 endorsementConfig;

    constructor() {}

    function initialize(
        bytes32 transactionId,
        address domain,
        bytes calldata config
    ) public initializer {
        bytes4 configSelector = bytes4(config[0:4]);
        if (configSelector == PenteConfigID_Endorsement_V0) {
            endorsementConfig = abi.decode(config[4:], (EndorsementConfig_V0));
        } else {
            revert PenteUnsupportedConfigType(configSelector);
        }

        emit PaladinNewSmartContract_V0(
            transactionId,
            domain,
            config
        );
    }

    function _authorizeUpgrade(
        address newImplementation
    ) internal view override {
        if (newImplementation != _nextImplementation) {
            revert PenteUpgradeNotPreauthorized();
        }
    }

    function preAuthorizeUpgrade(
        address newImplementation,
        bytes[] calldata signatures
    ) public {
        bytes32 upgradeHash = _buildUpgradeHash(newImplementation);
        validateEndorsements(upgradeHash, signatures);
        _nextImplementation = newImplementation;
    }

    function transition(
        bytes32            txID,
        bytes32[] calldata accounts,
        bytes32[] calldata oldStates,
        bytes32[] calldata newStates,
        bytes[]   calldata signatures
    ) public {
        bytes32 transitionHash = _buildTransitionHash(accounts, oldStates, newStates);
        validateEndorsements(transitionHash, signatures);

        // Perform the state transitions
        for (uint i = 0; i < accounts.length; i++) {
            if (oldStates[i] != _accountStates[accounts[i]]) {
                revert PenteAccountStateMismatch(oldStates[i], _accountStates[accounts[i]]);
            }
            _accountStates[accounts[i]] = newStates[i];
        }

        // Emmit the state transition event
        emit PaladinPrivateTransaction_V0(txID, oldStates, newStates, new bytes(0));
    }

    function _buildTransitionHash(
        bytes32[] calldata accounts,
        bytes32[] calldata oldStates,
        bytes32[] calldata newStates
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(
                TRANSITION_TYPEHASH,
                keccak256(abi.encodePacked(accounts)),
                keccak256(abi.encodePacked(oldStates)),
                keccak256(abi.encodePacked(newStates))
            )
        );
        return _hashTypedDataV4(structHash);
    }

    function _buildUpgradeHash(
        address newImplementation
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(
                UPGRADE_TYPEHASH,
                newImplementation
            )
        );
        return _hashTypedDataV4(structHash);
    }

    function validateEndorsements(
        bytes32 txHash,
        bytes[] calldata
        signatures
    ) internal view {
        address[] memory endorsers = endorsementConfig.endorsmentSet;
        address[] memory endorsements = new address[](signatures.length);
        uint collected;
        for (uint iSig = 0; iSig < signatures.length; iSig++) {
            // Check this signer is a new one
            address signer = ECDSA.recover(txHash, signatures[iSig]);
            for (uint iExist = 0; iExist < collected; iExist++) {
                if (signer == endorsements[iExist]) {
                    revert PenteDuplicateEndorser(signer);
                }
            }
            // Check this signer is in the endorser list
            address endorser;
            for (uint iEndorser = 0; iEndorser < endorsers.length; iEndorser++) {
                if (signer == endorsers[iEndorser]) {
                    endorser = endorsers[iEndorser];
                    break;
                }
            }
            if (endorser == address(0)) {
                revert PenteInvalidEndorser(signer);
            }
            // Add them to the list
            endorsements[collected] = endorser;
            collected++;
        }
        // Check we reached our threshold
        if (collected < endorsementConfig.threshold) {
            revert PenteEndorsementThreshold(collected, endorsementConfig.threshold);
        }
    }

}
