// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {EIP712Upgradeable} from "@openzeppelin/contracts-upgradeable/utils/cryptography/EIP712Upgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {INoto} from "../interfaces/INoto.sol";
import {INotoErrors} from "../interfaces/INotoErrors.sol";

/**
 * @title Noto
 * @author Kaleido, Inc.
 * @dev An implementation of a Confidential UTXO (C-UTXO) pattern, with participant
 *      confidentiality and anonymity based on notary submission and validation of
 *      transactions.
 *
 *      The EVM ledger provides double-spend protection on the transaction outputs,
 *      and provides a deterministic linkage (a DAG) of inputs and outputs.
 *
 *      The notary must authorize every transaction by either:
 *        1. Submitting the transaction directly
 *        2. Creating a "lock" on a proposed transaction and then delegating it
 *           to be spent (executed) by another EVM address
 */
contract Noto is EIP712Upgradeable, UUPSUpgradeable, INoto, INotoErrors {
    struct NotoConfig_V1 {
        string name;
        string symbol;
        uint8 decimals;
        address notary;
        uint64 variant;
        bytes data;
    }

    struct LockInfo {
        address owner;
        address delegate;
        bytes32 unlockHash;
        bytes32 unlockTxId;
    }

    // Config follows the convention of a 4 byte type selector, followed by ABI encoded bytes
    bytes4 public constant NotoConfigID_V1 = 0x00020000;

    uint64 public constant NotoVariantDefault = 0x0001;

    bytes32 private constant UNLOCK_TYPEHASH =
        keccak256(
            "Unlock(bytes32[] lockedInputs,bytes32[] lockedOutputs,bytes32[] outputs,bytes data)"
        );

    string private _name;
    string private _symbol;
    address public notary;

    mapping(bytes32 => bool) private _unspent;
    mapping(bytes32 => bool) private _txIds;

    mapping(bytes32 => bytes32) private _locked; // state ID => lock ID
    mapping(bytes32 => LockInfo) private _locks; // lock ID => lock info
    mapping(bytes32 => bytes32) private _lockTxIds; // tx ID => lock ID (for prepared transactions)

    function requireNotary(address addr) internal view {
        if (addr != notary) {
            revert NotoNotNotary(addr);
        }
    }

    modifier onlyNotary() {
        requireNotary(msg.sender);
        _;
    }

    modifier txIdNotUsed(bytes32 txId) {
        if (_txIds[txId]) {
            revert NotoDuplicateTransaction(txId);
        }
        _txIds[txId] = true;
        _;
    }

    function requireLockDelegate(bytes32 lockId, address addr) internal view {
        address delegate = _locks[lockId].delegate;
        if (addr != delegate) {
            revert NotoInvalidDelegate(lockId, delegate, addr);
        }
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(
        string memory name_,
        string memory symbol_,
        address notary_
    ) public virtual initializer {
        __EIP712_init("noto", "0.0.1");
        _name = name_;
        _symbol = symbol_;
        notary = notary_;
    }

    function buildConfig(
        bytes calldata data
    ) external view returns (bytes memory) {
        return
            _encodeConfig(
                NotoConfig_V1({
                    name: _name,
                    symbol: _symbol,
                    decimals: decimals(),
                    notary: notary,
                    variant: NotoVariantDefault,
                    data: data
                })
            );
    }

    function _encodeConfig(
        NotoConfig_V1 memory config
    ) internal pure returns (bytes memory) {
        bytes memory configOut = abi.encode(
            config.name,
            config.symbol,
            config.decimals,
            config.notary,
            config.variant,
            config.data
        );
        return bytes.concat(NotoConfigID_V1, configOut);
    }

    function _authorizeUpgrade(address) internal override onlyNotary {}

    /**
     * @dev Returns the name of the token.
     */
    function name() external view returns (string memory) {
        return _name;
    }

    /**
     * @dev Returns the symbol of the token.
     */
    function symbol() external view returns (string memory) {
        return _symbol;
    }

    /**
     * @dev Returns the decimals places of the token.
     */
    function decimals() public pure returns (uint8) {
        return 4;
    }

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
        return _locked[id] != bytes32(0);
    }

    /**
     * @dev Query the lockId for a locked state.
     * @param id The state identifier.
     * @return lockId The lockId set when the lock was created, or bytes32(0) if not locked.
     */
    function getLockId(bytes32 id) public view returns (bytes32 lockId) {
        return _locked[id];
    }

    /**
     * @dev Get current information about a lock.
     * @param lockId The identifier of the lock.
     * @return info The information about the lock.
     */
    function getLock(
        bytes32 lockId
    ) public view returns (LockInfo memory info) {
        return _locks[lockId];
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
            if (isUnspent(outputs[i]) || getLockId(outputs[i]) != bytes32(0)) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _unspent[outputs[i]] = true;
        }
    }

    function _buildUnlockHash(
        bytes32[] memory lockedInputs,
        bytes32[] memory lockedOutputs,
        bytes32[] memory outputs,
        bytes memory data
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
     * @dev Compute the lockId deterministically from a transaction ID.
     *      This allows callers to predict the lockId before calling createLock/createMintLock.
     *
     * @param txId The transaction ID that will be used to create the lock.
     * @return lockId The computed unique identifier for the lock.
     */
    function computeLockId(bytes32 txId) public view returns (bytes32) {
        return keccak256(abi.encode(address(this), msg.sender, txId));
    }

    /**
     * @dev Lock some value so it cannot be spent until it is unlocked.
     *      The lockId is computed deterministically from the txId.
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
        bytes32 lockId = computeLockId(txId);
        if (_locks[lockId].owner != address(0)) {
            revert NotoDuplicateLock(lockId);
        }

        _processInputs(inputs);
        _processOutputs(outputs);
        _processLockedOutputs(lockId, lockedOutputs);

        LockInfo storage lockInfo = _locks[lockId];
        lockInfo.owner = msg.sender;

        emit NotoLock(
            txId,
            lockId,
            inputs,
            outputs,
            lockedOutputs,
            signature,
            data
        );
    }

    /**
     * @dev Unlock some value from a set of locked states.
     *      May be triggered by the notary (if lock is undelegated) or by the current lock delegate.
     *      If triggered by the lock delegate, only a prepared unlock operation may be triggered.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param lockId the lock ID to unlock
     * @param params the parameters for the unlock operation
     *               - lockedInputs: array of zero or more locked outputs of a previous function call
     *               - lockedOutputs: array of zero or more locked outputs to generate, which will be tied to the lock ID
     *               - outputs: array of zero or more new unlocked outputs to generate, for future transactions to spend
     *               - signature: a signature over the original request to the notary (opaque to the blockchain)
     *               - data: any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoUnlock} event.
     */
    function unlock(
        bytes32 txId,
        bytes32 lockId,
        UnlockParams memory params
    ) external virtual override txIdNotUsed(txId) {
        LockInfo storage lockInfo = _locks[lockId];

        _validateUnlock(
            lockId,
            lockInfo,
            params.lockedInputs,
            params.lockedOutputs,
            params.outputs,
            params.data
        );
        lockInfo.unlockHash = bytes32(0);
        lockInfo.delegate = address(0);

        _processLockedInputs(lockId, params.lockedInputs);
        _processLockedOutputs(lockId, params.lockedOutputs);
        _processOutputs(params.outputs);

        emit NotoUnlock(
            txId,
            lockId,
            msg.sender,
            params.lockedInputs,
            params.lockedOutputs,
            params.outputs,
            params.signature,
            params.data
        );
    }

    function _validateUnlock(
        bytes32 lockId,
        LockInfo storage lockInfo,
        bytes32[] memory lockedInputs,
        bytes32[] memory lockedOutputs,
        bytes32[] memory outputs,
        bytes memory data
    ) internal view {
        if (lockInfo.delegate == address(0)) {
            requireNotary(msg.sender);
        } else {
            requireLockDelegate(lockId, msg.sender);

            if (lockInfo.unlockHash != 0) {
                bytes32 actualHash = _buildUnlockHash(
                    lockedInputs,
                    lockedOutputs,
                    outputs,
                    data
                );
                if (actualHash != lockInfo.unlockHash) {
                    revert NotoInvalidUnlockHash(lockInfo.unlockHash, actualHash);
                }
            }
        }
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
        bytes32 txId,
        bytes32 lockId,
        bytes32 unlockTxId,
        bytes32[] calldata lockedInputs,
        bytes32 unlockHash,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        LockInfo storage lockInfo = _locks[lockId];
        if (lockInfo.delegate != address(0)) {
            revert NotoAlreadyPrepared(unlockHash);
        }
        if (lockInfo.unlockHash != 0) {
            revert NotoAlreadyPrepared(unlockHash);
        }
        if (_txIds[unlockTxId]) {
            revert NotoDuplicateTransaction(unlockTxId);
        }

        _checkLockedInputs(lockId, lockedInputs);
        lockInfo.unlockHash = unlockHash;
        _lockTxIds[unlockTxId] = lockId;

        emit NotoUnlockPrepared(
            txId,
            lockId,
            unlockTxId,
            lockedInputs,
            unlockHash,
            signature,
            data
        );
    }

    /**
     * @dev Change the current delegate for a lock.
     *      May be triggered by the notary (if lock is undelegated) or by the current lock delegate.
     *      May only be triggered after an unlock operation has been prepared.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param lockId the lock ID to delegate
     * @param delegate the address that is authorized to perform the unlock
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoLockDelegated} event.
     */
    function delegateLock(
        bytes32 txId,
        bytes32 lockId,
        address delegate,
        bytes calldata signature,
        bytes calldata data
    ) external virtual txIdNotUsed(txId) {
        LockInfo storage lockInfo = _locks[lockId];
        address currentDelegate = lockInfo.delegate;
        if (currentDelegate == address(0)) {
            requireNotary(msg.sender);
        } else {
            requireLockDelegate(lockId, msg.sender);
        }
        lockInfo.delegate = delegate;
        bytes32 unlockHash = lockInfo.unlockHash;
        emit NotoLockDelegated(
            txId,
            lockId,
            unlockHash,
            delegate,
            signature,
            data
        );
    }

    /**
     * @dev Check the inputs are all locked.
     */
    function _checkLockedInputs(
        bytes32 lockId,
        bytes32[] memory inputs
    ) internal view {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (_locked[inputs[i]] != lockId) {
                revert NotoInvalidInput(inputs[i]);
            }
        }
    }

    /**
     * @dev Check the inputs are all locked, and remove them.
     */
    function _processLockedInputs(
        bytes32 lockId,
        bytes32[] memory inputs
    ) internal {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (_locked[inputs[i]] != lockId) {
                revert NotoInvalidInput(inputs[i]);
            }
            delete _locked[inputs[i]];
        }
    }

    /**
     * @dev Check the outputs are all new, and mark them as locked.
     */
    function _processLockedOutputs(
        bytes32 lockId,
        bytes32[] memory outputs
    ) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (isUnspent(outputs[i]) || getLockId(outputs[i]) != bytes32(0)) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _locked[outputs[i]] = lockId;
        }
    }
}
