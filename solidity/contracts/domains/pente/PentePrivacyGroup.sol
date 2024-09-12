// SPDX-License-Identifier: Apache-2.0
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
        keccak256("Transition(bytes32[] inputs,bytes32[] reads,bytes32[] outputs)");
    bytes32 private constant UPGRADE_TYPEHASH =
        keccak256("Upgrade(address address)");

    mapping(bytes32 => bool) private _unspent;
    address _nextImplementation;

    // Config follows the convention of a 4 byte type selector, followed by ABI encoded bytes
    bytes4 public constant PenteConfigID_Endorsement_V0 = 0x00010000;

    error PenteUnsupportedConfigType(bytes4 configSelector);
    error PenteDuplicateEndorser(address signer);
    error PenteInvalidEndorser(address signer);
    error PenteEndorsementThreshold(uint obtained, uint required);
    error PenteUpgradeNotPreauthorized();
    error PenteInputNotAvailable(bytes32 input);
    error PenteReadNotAvailable(bytes32 read);
    error PenteOutputAlreadyUnspent(bytes32 output);

    struct EndorsementConfig {
        uint      threshold;
        address[] endorsmentSet;
    }

    EndorsementConfig _endorsementConfig;

    constructor() {}

    function initialize(
        bytes32 transactionId,
        address domain,
        bytes calldata config
    ) public initializer {
        __EIP712_init("pente", "0.0.1");

        bytes4 configSelector = bytes4(config[0:4]);
        if (configSelector == PenteConfigID_Endorsement_V0) {
            ( /* string memory evmVersion */, uint threshold, address[] memory endorsmentSet) = 
                abi.decode(config[4:], (string, uint, address[]));
            _endorsementConfig = EndorsementConfig({
                threshold: threshold,
                endorsmentSet: endorsmentSet
            });
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
        bytes32[] calldata inputs,
        bytes32[] calldata reads,
        bytes32[] calldata outputs,
        bytes[]   calldata signatures
    ) public {
        bytes32 transitionHash = _buildTransitionHash(inputs, reads, outputs);
        validateEndorsements(transitionHash, signatures);

        // Perform the state transitions
        for (uint i = 0; i < inputs.length; i++) {
            if (!_unspent[inputs[i]]) {
                revert PenteInputNotAvailable(inputs[i]);
            }
            delete(_unspent[inputs[i]]);
        }
        for (uint i = 0; i < reads.length; i++) {
            if (!_unspent[reads[i]]) {
                revert PenteReadNotAvailable(reads[i]);
            }
        }
        for (uint i = 0; i < outputs.length; i++) {
            if (_unspent[outputs[i]]) {
                revert PenteOutputAlreadyUnspent(outputs[i]);
            }
            _unspent[outputs[i]] = true;
        }

        // Emmit the state transition event
        emit PaladinPrivateTransaction_V0(txID, inputs, outputs, new bytes(0));
    }

    function _buildTransitionHash(
        bytes32[] calldata inputs,
        bytes32[] calldata reads,
        bytes32[] calldata outputs
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(
                TRANSITION_TYPEHASH,
                keccak256(abi.encodePacked(inputs)),
                keccak256(abi.encodePacked(reads)),
                keccak256(abi.encodePacked(outputs))
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
        address[] memory endorsers = _endorsementConfig.endorsmentSet;
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
        if (collected < _endorsementConfig.threshold) {
            revert PenteEndorsementThreshold(collected, _endorsementConfig.threshold);
        }
    }

}
