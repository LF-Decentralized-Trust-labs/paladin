// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Address} from "@openzeppelin/contracts/utils/Address.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {EIP712Upgradeable} from "@openzeppelin/contracts-upgradeable/utils/cryptography/EIP712Upgradeable.sol";
import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {IPente} from "../interfaces/IPente.sol";

/// @title Base smart contract for pinning privacy group endorsed private smart
///      contract state transitions to the base ledger.
/// @author Kaleido, Inc.
/// @dev Each transition is a UTXO transaction:
///      - Spending the previous account state of the privacy group (private nonces)
///      - Spending the previous account state of smart contracts in this privacy group
///      - TODO: Spending privacy group / account states on accounts in other other privacy groups atomically
///
contract PentePrivacyGroup is IPente, UUPSUpgradeable, EIP712Upgradeable {
    using Address for address;

    string private constant TRANSITION_TYPE =
        "Transition(bytes32[] inputs,bytes32[] reads,bytes32[] outputs,bytes32[] info,ExternalCall[] externalCalls)";
    string private constant EXTERNALCALL_TYPE =
        "ExternalCall(address contractAddress,bytes encodedCall)";
    bytes32 private constant TRANSITION_TYPEHASH =
        keccak256(abi.encodePacked(TRANSITION_TYPE, EXTERNALCALL_TYPE));
    bytes32 private constant EXTERNALCALL_TYPEHASH =
        keccak256(abi.encodePacked(EXTERNALCALL_TYPE));

    bytes32 private constant UPGRADE_TYPEHASH =
        keccak256("Upgrade(address address)");

    mapping(bytes32 => bool) private _unspent;
    mapping(bytes32 => address) private _approvals;
    mapping(bytes32 => bool) private _txids;
    address _nextImplementation;

    // Config follows the convention of a 4 byte type selector, followed by ABI encoded bytes
    bytes4 public constant PenteConfig_V0 = 0x00010000;

    error PenteDuplicateTransaction(bytes32 txId);
    error PenteUnsupportedConfigType(bytes4 configSelector);
    error PenteDuplicateEndorser(address signer);
    error PenteInvalidEndorser(address signer);
    error PenteEndorsementThreshold(uint obtained, uint required);
    error PenteUpgradeNotPreauthorized();
    error PenteInputNotAvailable(bytes32 input);
    error PenteReadNotAvailable(bytes32 read);
    error PenteOutputAlreadyUnspent(bytes32 output);
    error PenteExternalCallsDisabled();
    error PenteInvalidDelegate(
        bytes32 txhash,
        address delegate,
        address sender
    );

    struct EndorsementConfig {
        uint threshold;
        address[] endorsementSet;
    }

    EndorsementConfig _endorsementConfig;
    bool _externalCallsEnabled;

    struct ExternalCall {
        address contractAddress;
        bytes encodedCall;
    }

    struct States {
        bytes32[] inputs;
        bytes32[] reads;
        bytes32[] outputs;
        bytes32[] info;
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(bytes calldata config) public initializer {
        __EIP712_init("pente", "0.0.1");

        bytes4 configSelector = bytes4(config[0:4]);
        if (configSelector == PenteConfig_V0) {
            (
                /*string memory evmVersion*/,
                uint threshold,
                address[] memory endorsementSet,
                uint externalCallsEnabled
            ) = abi.decode(config[4:], (string, uint, address[], uint));
            _endorsementConfig = EndorsementConfig({
                threshold: threshold,
                endorsementSet: endorsementSet
            });
            _externalCallsEnabled = externalCallsEnabled == 0 ? false : true;
        } else {
            revert PenteUnsupportedConfigType(configSelector);
        }
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
        bytes32 txId,
        States calldata states,
        ExternalCall[] calldata externalCalls,
        bytes[] calldata signatures
    ) external {
        bytes32 transitionHash = _buildTransitionHash(states, externalCalls);
        validateEndorsements(transitionHash, signatures);
        _transition(txId, states, externalCalls);
    }

    function approveTransition(
        bytes32 txId,
        address delegate,
        bytes32 transitionHash,
        bytes[] calldata signatures
    ) external {
        validateEndorsements(transitionHash, signatures);
        _approvals[transitionHash] = delegate;
        emit PenteApproved(txId, delegate, transitionHash);
    }

    function transitionWithApproval(
        bytes32 txId,
        States calldata states,
        ExternalCall[] calldata externalCalls
    ) external {
        bytes32 transitionHash = _buildTransitionHash(states, externalCalls);
        if (_approvals[transitionHash] != msg.sender) {
            revert PenteInvalidDelegate(
                transitionHash,
                _approvals[transitionHash],
                msg.sender
            );
        }
        _transition(txId, states, externalCalls);
    }

    function _transition(
        bytes32 txId,
        States calldata states,
        ExternalCall[] calldata externalCalls
    ) internal {
        // On-chain enforcement of TXID uniqueness
        if (_txids[txId] != false) {
            revert PenteDuplicateTransaction(txId);
        }
        _txids[txId] = true;

        // Perform the state transitions
        for (uint i = 0; i < states.inputs.length; i++) {
            if (!_unspent[states.inputs[i]]) {
                revert PenteInputNotAvailable(states.inputs[i]);
            }
            delete _unspent[states.inputs[i]];
        }
        for (uint i = 0; i < states.reads.length; i++) {
            if (!_unspent[states.reads[i]]) {
                revert PenteReadNotAvailable(states.reads[i]);
            }
        }
        for (uint i = 0; i < states.outputs.length; i++) {
            if (_unspent[states.outputs[i]]) {
                revert PenteOutputAlreadyUnspent(states.outputs[i]);
            }
            _unspent[states.outputs[i]] = true;
        }

        // Emit the state transition event
        emit PenteTransition(
            txId,
            states.inputs,
            states.reads,
            states.outputs,
            states.info
        );

        // Trigger any external calls
        for (uint i = 0; i < externalCalls.length; i++) {
            _invokeExternal(
                externalCalls[i].contractAddress,
                externalCalls[i].encodedCall
            );
        }
    }

    function _buildTransitionHash(
        States calldata states,
        ExternalCall[] calldata externalCalls
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(
                TRANSITION_TYPEHASH,
                keccak256(abi.encodePacked(states.inputs)),
                keccak256(abi.encodePacked(states.reads)),
                keccak256(abi.encodePacked(states.outputs)),
                keccak256(abi.encodePacked(states.info)),
                _buildExternalCallsHash(externalCalls)
            )
        );
        return _hashTypedDataV4(structHash);
    }

    function _buildExternalCallsHash(
        ExternalCall[] calldata externalCalls
    ) internal pure returns (bytes32) {
        bytes32[] memory hashes = new bytes32[](externalCalls.length);
        for (uint i = 0; i < externalCalls.length; i++) {
            hashes[i] = keccak256(
                abi.encode(
                    EXTERNALCALL_TYPEHASH,
                    externalCalls[i].contractAddress,
                    keccak256(externalCalls[i].encodedCall)
                )
            );
        }
        return keccak256(abi.encodePacked(hashes));
    }

    function _buildUpgradeHash(
        address newImplementation
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(UPGRADE_TYPEHASH, newImplementation)
        );
        return _hashTypedDataV4(structHash);
    }

    function validateEndorsements(
        bytes32 txHash,
        bytes[] calldata signatures
    ) internal view {
        address[] memory endorsers = _endorsementConfig.endorsementSet;
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
            for (
                uint iEndorser = 0;
                iEndorser < endorsers.length;
                iEndorser++
            ) {
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
            revert PenteEndorsementThreshold(
                collected,
                _endorsementConfig.threshold
            );
        }
    }

    function _invokeExternal(
        address contractAddress,
        bytes memory encodedCall
    ) internal {
        if (!_externalCallsEnabled) {
            revert PenteExternalCallsDisabled();
        }
        contractAddress.functionCall(encodedCall);
    }
}
