// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {EIP712Upgradeable} from "@openzeppelin/contracts-upgradeable/utils/cryptography/EIP712Upgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {INoto} from "../interfaces/INoto.sol";

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
contract Noto is EIP712Upgradeable, UUPSUpgradeable, INoto {
    address _notary;
    mapping(bytes32 => bool) private _unspent;
    mapping(bytes32 => address) private _approvals;
    mapping(bytes32 => LockDetail) private _locks;

    error NotoInvalidInput(bytes32 id);
    error NotoInvalidOutput(bytes32 id);
    error NotoNotNotary(address sender);
    error NotoInvalidDelegate(bytes32 txhash, address delegate, address sender);

    error NotoLockNotFound(bytes32 lockId);
    error NotoInvalidLockDelegate(
        bytes32 lockId,
        address delegate,
        address sender
    );
    error NotoInvalidUnlockHash(
        bytes32 lockId,
        bytes32 expected,
        bytes32 actual
    );

    struct LockDetail {
        uint256 stateCount;
        mapping(bytes32 => bool) states;
        bytes32 unlock;
        address delegate;
    }

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

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(
        address notaryAddress,
        bytes calldata data
    ) public virtual initializer returns (bytes memory) {
        __EIP712_init("noto", "0.0.1");
        _notary = notaryAddress;

        return
            _encodeConfig(
                NotoConfig_V0({
                    notaryAddress: notaryAddress,
                    data: data,
                    variant: NotoVariantDefault
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
     * @param inputs array of zero or more outputs of a previous function call against this
     *      contract that have not yet been spent, and the signer is authorized to spend
     * @param outputs array of zero or more new outputs to generate, for future transactions to spend
     * @param signature EIP-712 signature on the original request that spawned this transaction
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {UTXOTransfer} event.
     */
    function transfer(
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary {
        _transfer(inputs, outputs, signature, data);
    }

    /**
     * @dev Perform a transfer with no input states. Base implementation is identical
     *      to transfer(), but both methods can be overriden to provide different constraints.
     */
    function mint(
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary {
        bytes32[] memory inputs;
        _transfer(inputs, outputs, signature, data);
    }

    function _transfer(
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes calldata signature,
        bytes calldata data
    ) internal {
        _processInputs(inputs);
        _processOutputs(outputs);
        emit NotoTransfer(inputs, outputs, signature, data);
    }

    /**
     * @dev Check the inputs are all existing unspent ids
     */
    function _processInputs(bytes32[] memory inputs) internal {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (_unspent[inputs[i]] == false) {
                revert NotoInvalidInput(inputs[i]);
            }
            delete (_unspent[inputs[i]]);
        }
    }

    /**
     * @dev Check the outputs are all new unspent ids
     */
    function _processOutputs(bytes32[] memory outputs) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (_unspent[outputs[i]] == true) {
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
     * @param delegate the address that is authorized to submit the transaction
     * @param txhash the pre-calculated hash of the transaction that is delegated
     * @param signature EIP-712 signature on the original request that spawned this transaction
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoApproved} event.
     */
    function approveTransfer(
        address delegate,
        bytes32 txhash,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary {
        _approveTransfer(delegate, txhash, signature, data);
    }

    function _approveTransfer(
        address delegate,
        bytes32 txhash,
        bytes calldata signature,
        bytes calldata data
    ) internal {
        _approvals[txhash] = delegate;
        emit NotoApproved(delegate, txhash, signature, data);
    }

    /**
     * @dev Transfer via delegation - must be the approved delegate.
     *
     * @param inputs as per transfer()
     * @param outputs as per transfer()
     * @param signature as per transfer()
     * @param data as per transfer()
     *
     * Emits a {NotoTransfer} event.
     */
    function transferWithApproval(
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) public {
        bytes32 txhash = _buildTransferHash(inputs, outputs, data);
        if (_approvals[txhash] != msg.sender) {
            revert NotoInvalidDelegate(txhash, _approvals[txhash], msg.sender);
        }

        _transfer(inputs, outputs, signature, data);

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
     * @param lockId the unique identifier for this lock
     * @param inputs array of zero or more outputs of a previous function call against this
     *      contract that have not yet been spent, and the signer is authorized to spend
     * @param outputs array of zero or more new outputs to generate, for future transactions to spend
     * @param lockedOutputs array of zero or more locked outputs to generate, which will be tied to the lock ID
     * @param signature EIP-712 signature on the original request that spawned this transaction
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoLock} event.
     */
    function lock(
        bytes32 lockId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes32[] calldata lockedOutputs,
        bytes calldata signature,
        bytes calldata data
    ) public virtual override onlyNotary {
        _processInputs(inputs);
        _processOutputs(outputs);
        _processLockedOutputs(lockId, lockedOutputs);
        emit NotoLock(lockId, inputs, outputs, lockedOutputs, signature, data);
    }

    /**
     * @dev Unlock some value from a set of locked states.
     *
     * @param lockId the unique identifier for the lock
     * @param lockedInputs array of zero or more locked outputs of a previous function call
     * @param lockedOutputs array of zero or more locked outputs to generate, which will be tied to the lock ID
     * @param outputs array of zero or more new unlocked outputs to generate, for future transactions to spend
     * @param signature EIP-712 signature on the original request that spawned this transaction
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoUnlock} event.
     */
    function unlock(
        bytes32 lockId,
        bytes32[] calldata lockedInputs,
        bytes32[] calldata lockedOutputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary {
        _unlock(lockId, lockedInputs, lockedOutputs, outputs);
        emit NotoUnlock(
            lockId,
            lockedInputs,
            lockedOutputs,
            outputs,
            signature,
            data
        );
    }

    function _unlock(
        bytes32 lockId,
        bytes32[] calldata lockedInputs,
        bytes32[] calldata lockedOutputs,
        bytes32[] calldata outputs
    ) internal {
        LockDetail storage lock_ = _locks[lockId];
        if (lock_.stateCount == 0) {
            revert NotoLockNotFound(lockId);
        }

        _processLockedInputs(lockId, lockedInputs);
        _processLockedOutputs(lockId, lockedOutputs);
        _processOutputs(outputs);

        if (lock_.stateCount == 0) {
            delete _locks[lockId];
        }
    }

    /**
     * @dev Prepare an unlock operation that can be triggered later by the lock delegate.
     *
     * @param lockId the unique identifier for the lock
     * @param lockedInputs array of zero or more locked outputs of a previous function call
     * @param unlockHash pre-calculated EIP-712 hash of the prepared unlock transaction
     * @param signature EIP-712 signature on the original request that spawned this transaction
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoUnlockPrepared} event.
     */
    function prepareUnlock(
        bytes32 lockId,
        bytes32[] calldata lockedInputs,
        bytes32 unlockHash,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary {
        LockDetail storage lock_ = _locks[lockId];
        if (lock_.stateCount == 0) {
            revert NotoLockNotFound(lockId);
        }
        if (lock_.delegate != address(0) && lock_.delegate != msg.sender) {
            revert NotoInvalidLockDelegate(lockId, lock_.delegate, msg.sender);
        }

        _checkLockedInputs(lockId, lockedInputs);

        lock_.unlock = unlockHash;

        emit NotoUnlockPrepared(
            lockId,
            lockedInputs,
            unlockHash,
            signature,
            data
        );
    }

    /**
     * @dev Change the current delegate for a lock.
     *
     * @param lockId the unique identifier for the lock
     * @param delegate the address that is authorized to perform the unlock
     * @param signature EIP-712 signature on the original request that spawned this transaction
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoLockDelegated} event.
     */
    function delegateLock(
        bytes32 lockId,
        address delegate,
        bytes calldata signature,
        bytes calldata data
    ) external virtual {
        LockDetail storage lock_ = _locks[lockId];
        if (lock_.stateCount == 0) {
            revert NotoLockNotFound(lockId);
        }
        if (lock_.delegate == address(0)) {
            requireNotary(msg.sender);
        } else if (lock_.delegate != msg.sender) {
            revert NotoInvalidLockDelegate(lockId, lock_.delegate, msg.sender);
        }

        lock_.delegate = delegate;

        emit NotoLockDelegated(lockId, delegate, signature, data);
    }

    /**
     * @dev Execute the prepared unlock operation as the authorized delegate.
     *
     * @param lockId as per unlock()
     * @param lockedInputs as per unlock()
     * @param lockedOutputs as per unlock()
     * @param outputs as per unlock()
     * @param data as per unlock()
     *
     * Emits a {NotoDelegateUnlock} event.
     */
    function unlockWithApproval(
        bytes32 lockId,
        bytes32[] calldata lockedInputs,
        bytes32[] calldata lockedOutputs,
        bytes32[] calldata outputs,
        bytes calldata data
    ) external virtual override {
        LockDetail storage lock_ = _locks[lockId];
        if (lock_.delegate != msg.sender) {
            revert NotoInvalidLockDelegate(lockId, lock_.delegate, msg.sender);
        }
        bytes32 unlockHash = _buildUnlockHash(
            lockedInputs,
            lockedOutputs,
            outputs,
            data
        );
        if (lock_.unlock != unlockHash) {
            revert NotoInvalidUnlockHash(lockId, lock_.unlock, unlockHash);
        }

        lock_.delegate = address(0);
        lock_.unlock = 0;

        _unlock(lockId, lockedInputs, lockedOutputs, outputs);
        emit NotoDelegateUnlock(
            lockId,
            msg.sender,
            lockedInputs,
            lockedOutputs,
            outputs,
            data
        );
    }

    /**
     * @dev Relinquish status as the lock delegate without executing the prepared unlock.
     *
     * @param lockId the unique identifier for the lock
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoUnlockApprovalCancelled} event.
     */
    function cancelUnlockApproval(
        bytes32 lockId,
        bytes calldata data
    ) external virtual override {
        LockDetail storage lock_ = _locks[lockId];
        if (lock_.delegate != msg.sender) {
            revert NotoInvalidLockDelegate(lockId, lock_.delegate, msg.sender);
        }
        lock_.delegate = address(0);
        emit NotoUnlockApprovalCancelled(lockId, data);
    }

    function _checkLockedInputs(
        bytes32 id,
        bytes32[] calldata inputs
    ) internal view {
        LockDetail storage lock_ = _locks[id];
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (lock_.states[inputs[i]] == false) {
                revert NotoInvalidInput(inputs[i]);
            }
        }
    }

    function _processLockedInputs(
        bytes32 id,
        bytes32[] calldata inputs
    ) internal {
        LockDetail storage lock_ = _locks[id];
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (lock_.states[inputs[i]] == false) {
                revert NotoInvalidInput(inputs[i]);
            }
            delete (lock_.states[inputs[i]]);
            lock_.stateCount--;
        }
    }

    function _processLockedOutputs(
        bytes32 id,
        bytes32[] calldata outputs
    ) internal {
        LockDetail storage lock_ = _locks[id];
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (lock_.states[outputs[i]] == true) {
                revert NotoInvalidOutput(outputs[i]);
            }
            lock_.states[outputs[i]] = true;
            lock_.stateCount++;
        }
    }
}
