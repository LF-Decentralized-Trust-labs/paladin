// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {EIP712Upgradeable} from "@openzeppelin/contracts-upgradeable/utils/cryptography/EIP712Upgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {INoto} from "../interfaces/INoto.sol";
import {INotoErrors} from "../interfaces/INotoErrors.sol";

/**
 * @title A sample on-chain implementation of a Confidential UTXO (C-UTXO) pattern,
 *        with participant confidentiality and anonymity based on notary submission and
 *        validation of transactions.
 * @author Kaleido, Inc.
 * @dev Transaction pre-verification is performed by a notary.
 *      The EVM ledger provides double-spend protection on the transaction outputs,
 *      and provides a deterministic linkage (a DAG) of inputs and outputs.
 *
 *      The notary must authorize every transaction by either:
 *
 *      1. Submitting the transaction directly
 *
 *      2. Pre-authorizing another EVM address to perform a transaction, by storing
 *         the EIP-712 typed-data hash of the transaction in an approval record.
 *         This allows coordination of DVP with other smart contracts, which could
 *         be using any model programmable via EVM (not just C-UTXO)
 */
contract Noto is EIP712Upgradeable, UUPSUpgradeable, INoto, INotoErrors {
    address _notary;
    mapping(bytes32 => bool) private _unspent;
    mapping(bytes32 => address) private _approvals;

    mapping(bytes32 => bool) private _locked;
    mapping(bytes32 => bytes32) private _unlockHashes; // state ID => unlock hash
    mapping(bytes32 => address) private _unlockDelegates; // unlock hash => delegate
    mapping(bytes32 => bool) private _txids;

    // Config follows the convention of a 4 byte type selector, followed by ABI encoded bytes
    bytes4 public constant NotoConfigID_V0 = 0x00010000;

    struct NotoConfig_V0 {
        address notaryAddress;
        uint64 variant;
        bytes data;
    }

    uint64 public constant NotoVariantDefault = 0x0000;

    bytes32 private constant TRANSFER_TYPEHASH =
        keccak256("Transfer(bytes32[] inputs,bytes32[] outputs,bytes data)");
    bytes32 private constant UNLOCK_TYPEHASH =
        keccak256(
            "Unlock(bytes32[] lockedInputs,bytes32[] lockedOutputs,bytes32[] outputs,bytes data)"
        );

    function requireNotary(address addr) internal view {
        if (addr != _notary) {
            revert NotoNotNotary(addr);
        }
    }

    modifier onlyNotary() {
        requireNotary(msg.sender);
        _;
    }

    modifier txIdNotUsed(bytes32 txId) {
        if (_txids[txId]) {
            revert NotoDuplicateTransaction(txId);
        }
        _txids[txId] = true;
        _;
    }

    function requireLockDelegate(
        bytes32 unlockHash,
        address addr
    ) internal view {
        if (addr != _unlockDelegates[unlockHash]) {
            revert NotoInvalidDelegate(
                unlockHash,
                _unlockDelegates[unlockHash],
                addr
            );
        }
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(address notaryAddress) public virtual initializer {
        __EIP712_init("noto", "0.0.1");
        _notary = notaryAddress;
    }

    function buildConfig(
        bytes calldata data
    ) external view returns (bytes memory) {
        return
            _encodeConfig(
                NotoConfig_V0({
                    notaryAddress: _notary,
                    variant: NotoVariantDefault,
                    data: data
                })
            );
    }

    function _encodeConfig(
        NotoConfig_V0 memory config
    ) internal pure returns (bytes memory) {
        bytes memory configOut = abi.encode(
            config.notaryAddress,
            config.variant,
            config.data
        );
        return bytes.concat(NotoConfigID_V0, configOut);
    }

    function _authorizeUpgrade(address) internal override onlyNotary {}

    /**
     * @dev query whether a TXO is currently in the unspent list
     * @param id the UTXO identifier
     * @return unspent true or false depending on whether the identifier is in the unspent map
     */
    function isUnspent(bytes32 id) public view returns (bool unspent) {
        return _unspent[id];
    }

    /**
     * @dev query whether a TXO is currently locked
     * @param id the UTXO identifier
     * @return locked true or false depending on whether the identifier is locked
     */
    function isLocked(bytes32 id) public view returns (bool locked) {
        return _locked[id];
    }

    /**
     * @dev query whether an approval exists for the given transaction
     * @param txhash the transaction hash
     * @return delegate the non-zero owner address, or zero if the TXO ID is not in the approval map
     */
    function getTransferApproval(
        bytes32 txhash
    ) public view returns (address delegate) {
        return _approvals[txhash];
    }

    /**
     * @dev The main function of the contract, which finalizes execution of a pre-verified
     *      transaction. The inputs and outputs are all opaque to this on-chain function.
     *      Provides ordering and double-spend protection.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param inputs array of zero or more outputs of a previous function call against this
     *      contract that have not yet been spent, and the signer is authorized to spend
     * @param outputs array of zero or more new outputs to generate, for future transactions to spend
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {UTXOTransfer} event.
     */
    function transfer(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary {
        _transfer(txId, inputs, outputs, signature, data);
    }

    /**
     * @dev Perform a transfer with no input states. Base implementation is identical
     *      to transfer(), but both methods can be overriden to provide different constraints.
     * @param txId a unique identifier for this transaction which must not have been used before
     */
    function mint(
        bytes32 txId,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary {
        bytes32[] memory inputs;
        _transfer(txId, inputs, outputs, signature, data);
    }

    function _transfer(
        bytes32 txId,
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes calldata signature,
        bytes calldata data
    ) internal txIdNotUsed(txId) {
        _processInputs(inputs);
        _processOutputs(outputs);
        emit NotoTransfer(txId, inputs, outputs, signature, data);
    }

    /**
     * @dev Check the inputs are all unspent, and remove them
     */
    function _processInputs(bytes32[] memory inputs) internal {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (!_unspent[inputs[i]]) {
                revert NotoInvalidInput(inputs[i]);
            }
            delete _unspent[inputs[i]];
        }
    }

    /**
     * @dev Check the outputs are all new, and mark them as unspent
     */
    function _processOutputs(bytes32[] memory outputs) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (_unspent[outputs[i]] || _locked[outputs[i]]) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _unspent[outputs[i]] = true;
        }
    }

    /**
     * @dev Authorize a transfer to be performed by another address in a future transaction
     *      (for example, a smart contract coordinating a DVP).
     *
     *      Note the txhash will only be spendable if it is exactly correct for
     *      the inputs/outputs/data that are later supplied in useDelegation.
     *      This approach is gas-efficient as it means:
     *      - The inputs/outputs/data are not stored on-chain at any point
     *      - The EIP-712 hash is only calculated on-chain once, in transferWithApproval()
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param delegate the address that is authorized to submit the transaction
     * @param txhash the pre-calculated hash of the transaction that is delegated
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoApproved} event.
     */
    function approveTransfer(
        bytes32 txId,
        address delegate,
        bytes32 txhash,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary txIdNotUsed(txId) {
        _approvals[txhash] = delegate;
        emit NotoApproved(txId, delegate, txhash, signature, data);
    }

    /**
     * @dev Transfer via delegation - must be the approved delegate.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param inputs as per transfer()
     * @param outputs as per transfer()
     * @param signature as per transfer()
     * @param data as per transfer()
     *
     * Emits a {NotoTransfer} event.
     */
    function transferWithApproval(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) public {
        bytes32 txhash = _buildTransferHash(inputs, outputs, data);
        if (_approvals[txhash] != msg.sender) {
            revert NotoInvalidDelegate(txhash, _approvals[txhash], msg.sender);
        }

        _transfer(txId, inputs, outputs, signature, data);

        delete _approvals[txhash];
    }

    function _buildTransferHash(
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata data
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(
                TRANSFER_TYPEHASH,
                keccak256(abi.encodePacked(inputs)),
                keccak256(abi.encodePacked(outputs)),
                keccak256(data)
            )
        );
        return _hashTypedDataV4(structHash);
    }

    function _buildUnlockHash(
        bytes32[] calldata lockedInputs,
        bytes32[] calldata lockedOutputs,
        bytes32[] calldata outputs,
        bytes calldata data
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(
                UNLOCK_TYPEHASH,
                keccak256(abi.encodePacked(lockedInputs)),
                keccak256(abi.encodePacked(lockedOutputs)),
                keccak256(abi.encodePacked(outputs)),
                keccak256(data)
            )
        );
        return _hashTypedDataV4(structHash);
    }

    /**
     * @dev Lock some value so it cannot be spent until it is unlocked.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param inputs array of zero or more outputs of a previous function call against this
     *      contract that have not yet been spent, and the signer is authorized to spend
     * @param outputs array of zero or more new outputs to generate, for future transactions to spend
     * @param lockedOutputs array of zero or more locked outputs to generate, which will be tied to the lock ID
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoLock} event.
     */
    function lock(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes32[] calldata lockedOutputs,
        bytes calldata signature,
        bytes calldata data
    ) public virtual override onlyNotary txIdNotUsed(txId) {
        _processInputs(inputs);
        _processOutputs(outputs);
        _processLockedOutputs(lockedOutputs);
        emit NotoLock(txId, inputs, outputs, lockedOutputs, signature, data);
    }

    /**
     * @dev Unlock some value from a set of locked states.
     *      May be triggered by the notary (if lock is undelegated) or by the current lock delegate.
     *      If triggered by the lock delegate, only a prepared unlock operation may be triggered.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param lockedInputs array of zero or more locked outputs of a previous function call
     * @param lockedOutputs array of zero or more locked outputs to generate, which will be tied to the lock ID
     * @param outputs array of zero or more new unlocked outputs to generate, for future transactions to spend
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoUnlock} event.
     */
    function unlock(
        bytes32 txId,
        bytes32[] calldata lockedInputs,
        bytes32[] calldata lockedOutputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override txIdNotUsed(txId) {
        bytes32 expectedHash;
        if (lockedInputs.length > 0) {
            expectedHash = _unlockHashes[lockedInputs[0]];
            delete _unlockHashes[lockedInputs[0]];
        }
        for (uint256 i = 1; i < lockedInputs.length; ++i) {
            if (_unlockHashes[lockedInputs[i]] != expectedHash) {
                revert NotoInvalidInput(lockedInputs[i]);
            }
            delete _unlockHashes[lockedInputs[i]];
        }

        address delegate = _unlockDelegates[expectedHash];
        if (delegate == address(0)) {
            requireNotary(msg.sender);
        } else {
            requireLockDelegate(expectedHash, msg.sender);
            delete _unlockDelegates[expectedHash];

            if (expectedHash != 0) {
                bytes32 actualHash = _buildUnlockHash(
                    lockedInputs,
                    lockedOutputs,
                    outputs,
                    data
                );
                if (actualHash != expectedHash) {
                    revert NotoInvalidUnlockHash(expectedHash, actualHash);
                }
            }
        }

        _processLockedInputs(lockedInputs);
        _processLockedOutputs(lockedOutputs);
        _processOutputs(outputs);

        emit NotoUnlock(
            txId,
            msg.sender,
            lockedInputs,
            lockedOutputs,
            outputs,
            signature,
            data
        );
    }

    /**
     * @dev Prepare an unlock operation that can be triggered later.
     *      May only be triggered by the notary, and only if the lock is not delegated.
     *
     * @param lockedInputs array of zero or more locked outputs of a previous function call
     * @param unlockHash pre-calculated EIP-712 hash of the prepared unlock transaction
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoUnlockPrepared} event.
     */
    function prepareUnlock(
        bytes32[] calldata lockedInputs,
        bytes32 unlockHash,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary {
        if (_unlockDelegates[unlockHash] != address(0)) {
            revert NotoAlreadyPrepared(unlockHash);
        }
        _checkLockedInputs(lockedInputs);
        for (uint256 i = 0; i < lockedInputs.length; ++i) {
            _unlockHashes[lockedInputs[i]] = unlockHash;
        }
        emit NotoUnlockPrepared(lockedInputs, unlockHash, signature, data);
    }

    /**
     * @dev Change the current delegate for a lock.
     *      May be triggered by the notary (if lock is undelegated) or by the current lock delegate.
     *      May only be triggered after an unlock operation has been prepared.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param delegate the address that is authorized to perform the unlock
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoLockDelegated} event.
     */
    function delegateLock(
        bytes32 txId,
        bytes32 unlockHash,
        address delegate,
        bytes calldata signature,
        bytes calldata data
    ) external virtual txIdNotUsed(txId) {
        address currentDelegate = _unlockDelegates[unlockHash];
        if (currentDelegate == address(0)) {
            requireNotary(msg.sender);
        } else {
            requireLockDelegate(unlockHash, msg.sender);
        }
        _unlockDelegates[unlockHash] = delegate;
        emit NotoLockDelegated(txId, unlockHash, delegate, signature, data);
    }

    /**
     * @dev Check the inputs are all locked
     */
    function _checkLockedInputs(bytes32[] calldata inputs) internal view {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (!_locked[inputs[i]]) {
                revert NotoInvalidInput(inputs[i]);
            }
        }
    }

    /**
     * @dev Check the inputs are all locked, and remove them
     */
    function _processLockedInputs(bytes32[] calldata inputs) internal {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (!_locked[inputs[i]]) {
                revert NotoInvalidInput(inputs[i]);
            }
            delete _locked[inputs[i]];
        }
    }

    /**
     * @dev Check the outputs are all new, and mark them as locked
     */
    function _processLockedOutputs(bytes32[] calldata outputs) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (_locked[outputs[i]] || _unspent[outputs[i]]) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _locked[outputs[i]] = true;
        }
    }
}
