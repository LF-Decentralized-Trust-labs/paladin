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

    struct NotoLock {
        // The base information about the lock
        LockInfo info;
        // The options for the lock
        LockOptions options;
        // The number of states locked by the lock
        uint256 stateCount;
    }

    // Config follows the convention of a 4 byte type selector, followed by ABI encoded bytes
    bytes4 public constant NotoConfigID_V1 = 0x00020000;

    uint64 public constant NotoVariantDefault = 0x0001;

    bytes32 private constant UNLOCK_TYPEHASH =
        keccak256(
            "Unlock(bytes32 txId,bytes32[] lockedInputs,bytes32[] outputs,bytes data)"
        );

    string private _name;
    string private _symbol;
    address public notary;

    mapping(bytes32 => bool) private _unspent;
    mapping(bytes32 => bytes32) private _locked; // state ID => lock ID
    mapping(bytes32 => NotoLock) private _locks; // lock ID => lock details
    mapping(bytes32 => bool) private _txIds; // track used transaction IDs
    mapping(bytes32 => bytes32) private _lockTxIds; // tx ID => lock ID (for prepared transactions)

    function requireNotary(address addr) internal view {
        if (addr != notary) {
            revert NotoNotNotary(addr);
        }
    }

    function requireSpender(bytes32 lockId, address addr) internal view {
        address spender = _locks[lockId].info.spender;
        if (addr != spender) {
            revert LockUnauthorized(lockId, addr);
        }
    }

    function useTxId(bytes32 txId) internal {
        if (_txIds[txId]) {
            revert NotoDuplicateTransaction(txId);
        }
        _txIds[txId] = true;
    }

    modifier onlyNotary() {
        requireNotary(msg.sender);
        _;
    }

    modifier onlySpender(bytes32 lockId) {
        requireSpender(lockId, msg.sender);
        _;
    }

    modifier onlyNotaryOrSpender(bytes32 lockId) {
        NotoLock storage lock = _locks[lockId];
        bool isDelegated = lock.info.spender != lock.info.owner;

        if (isDelegated) {
            // Delegated locks can only be used by the spender
            requireSpender(lockId, msg.sender);
        } else {
            // Undelegated locks can only be used by the notary
            requireNotary(msg.sender);
        }
        _;
    }

    modifier txIdNotUsed(bytes32 txId) {
        useTxId(txId);
        _;
    }

    modifier lockActive(bytes32 lockId) {
        if (_locks[lockId].info.owner == address(0)) {
            revert LockNotActive(lockId);
        }
        _;
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
     * @dev Returns the number of decimals places of the token.
     */
    function decimals() public pure returns (uint8) {
        return 4;
    }

    /**
     * @dev Query whether a state is currently in the unspent list.
     * @param id The state identifier.
     * @return unspent True or false depending on whether the identifier is in the unspent map.
     */
    function isUnspent(bytes32 id) public view returns (bool unspent) {
        return _unspent[id];
    }

    /**
     * @dev Query the lockId for a locked state.
     * @param id The state identifier.
     * @return lockId The lockId set when the lock was created.
     */
    function getLockId(bytes32 id) public view returns (bytes32 lockId) {
        return _locked[id];
    }

    /**
     * @dev Query the options for a lock.
     * @param lockId The lockId set when the lock was created.
     * @return options The options for the lock.
     */
    function getLockOptions(
        bytes32 lockId
    ) public view returns (LockOptions memory options) {
        return _locks[lockId].options;
    }

    /**
     * @dev Get current information about a lock.
     *      Reverts if the lock is not active.
     *
     * @param lockId The identifier of the lock.
     * @return info The information about the lock.
     */
    function getLock(
        bytes32 lockId
    ) public view override lockActive(lockId) returns (LockInfo memory info) {
        return _locks[lockId].info;
    }

    /**
     * @dev Spend states and create new states.
     *
     * @param txId A unique identifier for this transaction which must not have been used before.
     * @param inputs Array of zero or more states that the signer is authorized to spend.
     * @param outputs Array of zero or more new states to generate, for future transactions to spend.
     * @param signature A signature over the original request to the notary (opaque to the blockchain).
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {Transfer} event.
     */
    function transfer(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        _transfer(txId, inputs, outputs, signature, data);
    }

    /**
     * @dev Perform a transfer with no input states. Base implementation is identical
     *      to transfer(), but both methods can be overridden to provide different constraints.
     *
     * @param txId A unique identifier for this transaction which must not have been used before.
     * @param outputs Array of zero or more new states to generate.
     * @param signature A signature over the original request to the notary (opaque to the blockchain).
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {Transfer} event.
     */
    function mint(
        bytes32 txId,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        _transfer(txId, new bytes32[](0), outputs, signature, data);
    }

    function _transfer(
        bytes32 txId,
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes calldata signature,
        bytes calldata data
    ) internal virtual {
        _processInputs(inputs);
        _processOutputs(outputs);
        emit Transfer(txId, msg.sender, inputs, outputs, signature, data);
    }

    /**
     * @dev Check that the inputs are all unspent, and remove them.
     */
    function _processInputs(bytes32[] memory inputs) internal {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (!isUnspent(inputs[i])) {
                revert NotoInvalidInput(inputs[i]);
            }
            delete _unspent[inputs[i]];
        }
    }

    /**
     * @dev Check that the outputs are all new, and mark them as unspent.
     */
    function _processOutputs(bytes32[] memory outputs) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (isUnspent(outputs[i]) || getLockId(outputs[i]) != 0) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _unspent[outputs[i]] = true;
        }
    }

    function _unlockHash(
        bytes32 txId,
        bytes32[] memory lockedInputs,
        bytes32[] memory outputs,
        bytes memory data
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(
                UNLOCK_TYPEHASH,
                txId,
                keccak256(abi.encodePacked(lockedInputs)),
                keccak256(abi.encodePacked(outputs)),
                keccak256(data)
            )
        );
        return _hashTypedDataV4(structHash);
    }

    /**
     * @dev Compute the lockId for given parameters.
     *      This allows callers to predict the lockId before calling createLock().
     *
     * @param params The lock parameters (only txId is used for lockId computation).
     * @return lockId The computed unique identifier for the lock.
     */
    function computeLockId(
        LockParams calldata params
    ) public view override returns (bytes32) {
        return _computeLockId(params.txId);
    }

    function _computeLockId(bytes32 txId) internal view returns (bytes32) {
        return keccak256(abi.encode(address(this), msg.sender, txId));
    }

    /**
     * @dev Create a new lock, by spending states and creating new locked states.
     *      Locks are identified by a unique lockId, which is generated deterministically.
     *      Locked states can be spent using spendLock(), or control of the lock can be
     *      delegated using delegateLock().
     *
     * @param params The lock parameters (see LockParams struct - note that inputs must be only unlocked states).
     * @param data Any additional transaction data (opaque to the blockchain).
     * @return lockId The generated unique identifier for the lock.
     *
     * Emits a {LockCreated} event.
     */
    function createLock(
        LockParams calldata params,
        bytes calldata data
    )
        external
        virtual
        override
        onlyNotary
        txIdNotUsed(params.txId)
        returns (bytes32)
    {
        bytes32 lockId = computeLockId(params);
        if (_locks[lockId].info.owner != address(0)) {
            revert NotoDuplicateLock(lockId);
        }
        _createLock(params, lockId, data);
        return lockId;
    }

    function _createLock(
        LockParams memory params,
        bytes32 lockId,
        bytes calldata data
    ) internal virtual {
        _processInputs(params.inputs);
        _processOutputs(params.outputs);
        _processLockedOutputs(lockId, params.lockedOutputs);

        // Initially, owner and spender are both the notary
        NotoLock storage lock = _locks[lockId];
        lock.info = LockInfo({
            owner: msg.sender,
            spender: msg.sender
        });
        lock.stateCount = params.lockedOutputs.length;

        if (params.options.length > 0) {
            _setLockOptions(lockId, params.options);
        }

        emit LockCreated(
            params.txId,
            lockId,
            lock.info.owner,
            lock.info.spender,
            params.inputs,
            params.outputs,
            params.lockedOutputs,
            params.proof,
            data
        );
    }

    /**
     * @dev Update the current options for a lock.
     *      Only allowed if the lock has not been prepared already.
     *
     * @param lockId Unique identifier for the lock.
     * @param params The update parameters (see UpdateLockParams struct).
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {LockUpdated} event.
     */
    function updateLock(
        bytes32 lockId,
        UpdateLockParams calldata params,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(params.txId) {
        NotoLock storage lock = _locks[lockId];
        if (lock.options.spendHash != 0 || lock.options.cancelHash != 0) {
            revert NotoAlreadyPrepared(lockId);
        }

        _checkLockedInputs(lockId, params.lockedInputs);
        _setLockOptions(lockId, params.options);

        emit LockUpdated(
            params.txId,
            lockId,
            msg.sender,
            params.lockedInputs,
            params.proof,
            data
        );
    }

    function _setLockOptions(
        bytes32 lockId,
        bytes memory options
    ) internal virtual {
        LockOptions memory lockOptions = abi.decode(options, (LockOptions));
        if (lockOptions.spendTxId == 0 || lockOptions.spendHash == 0 || lockOptions.cancelHash == 0) {
            revert NotoInvalidOptions(options);
        }
        if (_txIds[lockOptions.spendTxId]) {
            revert NotoDuplicateTransaction(lockOptions.spendTxId);
        }
        _locks[lockId].options = lockOptions;
        _lockTxIds[lockOptions.spendTxId] = lockId;
    }

    /**
     * @dev Consume ("spend") the capability represented by this lock.
     *      The caller will either be the notary, or a delegated spender.
     *      If the caller is a delegated spender, the lock must have been prepared with a spendHash,
     *      and the provided state data must match the spendHash.
     *
     * @param lockId The identifier of the lock.
     * @param data ABI-encoded UnlockData struct.
     *
     * Emits a {LockSpent} event.
     */
    function spendLock(
        bytes32 lockId,
        bytes calldata data
    ) external override lockActive(lockId) onlySpender(lockId) {
        NotoLock storage lock = _locks[lockId];
        _spendLock(lockId, lock.options.spendHash, data);
        emit LockSpent(lockId, msg.sender, data);
    }

    /**
     * @dev Cancel this lock without performing its effect.
     *      The caller will either be the notary, or a delegated spender.
     *      If the caller is a delegated spender, the lock must have been prepared with a cancelHash,
     *      and the provided state data must match the cancelHash.
     *
     * @param lockId The identifier of the lock.
     * @param data ABI-encoded UnlockData struct.
     *
     * Emits a {LockCancelled} event.
     */
    function cancelLock(
        bytes32 lockId,
        bytes calldata data
    ) external override lockActive(lockId) onlySpender(lockId) {
        NotoLock storage lock = _locks[lockId];
        _spendLock(lockId, lock.options.cancelHash, data);
        emit LockCancelled(lockId, msg.sender, data);
    }

    function _spendLock(
        bytes32 lockId,
        bytes32 expectedHash,
        bytes calldata data
    ) internal {
        UnlockData memory params;
        (params.txId, params.inputs, params.outputs, params.data) = abi.decode(
            data,
            (bytes32, bytes32[], bytes32[], bytes)
        );

        NotoLock storage lock = _locks[lockId];
        if (lock.options.spendTxId != 0 && lock.options.spendTxId != params.txId) {
            revert NotoInvalidTransaction(params.txId);
        }
        _txIds[params.txId] = true;

        // If a specific unlock operation is expected, verify the hash matches
        if (expectedHash != 0) {
            bytes32 actualHash = _unlockHash(
                params.txId,
                params.inputs,
                params.outputs,
                params.data
            );
            if (actualHash != expectedHash) {
                revert NotoInvalidUnlockHash(expectedHash, actualHash);
            }
        }

        // The operation must unlock all the locked inputs
        if (lock.stateCount != params.inputs.length) {
            revert NotoInvalidUnlockInputs(
                lock.stateCount,
                params.inputs.length
            );
        }

        _processLockedInputs(lockId, params.inputs);
        _processOutputs(params.outputs);

        delete _locks[lockId];
    }

    /**
     * @dev Delegate spending authority for a lock to a new address.
     *      The lock must have been prepared (both spendHash and cancelHash set).
     *
     * @param lockId The identifier of the lock.
     * @param newSpender The address of the new lock spender.
     * @param data ABI-encoded DelegateLockData struct.
     *
     * Emits a {LockDelegated} event.
     */
    function delegateLock(
        bytes32 lockId,
        address newSpender,
        bytes calldata data
    ) external override lockActive(lockId) onlySpender(lockId) {
        NotoLock storage lock = _locks[lockId];
        if (lock.options.spendHash == 0 || lock.options.cancelHash == 0) {
            revert NotoNotPrepared(lockId);
        }

        DelegateLockData memory params;
        (params.txId, params.data) = abi.decode(data, (bytes32, bytes));
        useTxId(params.txId);

        address previousSpender = lock.info.spender;
        lock.info.spender = newSpender;

        emit LockDelegated(lockId, previousSpender, newSpender, data);
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
            if (isUnspent(outputs[i]) || getLockId(outputs[i]) != 0) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _locked[outputs[i]] = lockId;
        }
    }
}
