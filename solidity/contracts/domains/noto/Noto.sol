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
    struct NotoConfig_V1 {
        string name;
        string symbol;
        uint8 decimals;
        address notary;
        uint64 variant;
        bytes data;
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
    mapping(bytes32 => bytes32) private _locked; // state ID => lock ID
    mapping(bytes32 => LockOptions) private _lockOptions; // lock ID => lock options
    mapping(bytes32 => address) private _lockDelegates; // lock ID => lock delegate
    mapping(bytes32 => bool) private _txids;

    function requireNotary(address addr) internal view {
        if (addr != notary) {
            revert NotoNotNotary(addr);
        }
    }

    function requireLockDelegate(bytes32 lockId, address addr) internal view {
        if (addr != _lockDelegates[lockId]) {
            revert NotoInvalidDelegate(lockId, _lockDelegates[lockId], addr);
        }
    }

    modifier onlyNotary() {
        requireNotary(msg.sender);
        _;
    }

    modifier onlyNotaryOrDelegate(bytes32 lockId) {
        if (_lockDelegates[lockId] == address(0)) {
            requireNotary(msg.sender);
        } else {
            requireLockDelegate(lockId, msg.sender);
        }
        _;
    }

    modifier txIdNotUsed(bytes32 txId) {
        if (_txids[txId]) {
            revert NotoDuplicateTransaction(txId);
        }
        _txids[txId] = true;
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
        return _locked[id] != 0;
    }

    /**
     * @dev query the lockId for a locked TXO
     * @param id the UTXO identifier
     */
    function getLockId(bytes32 id) public view returns (bytes32 lockId) {
        return _locked[id];
    }

    /**
     * @dev query the options for a lock
     * @param lockId the lockId set when the lock was created
     */
    function getLockOptions(
        bytes32 lockId
    ) public view returns (LockOptions memory options) {
        return _lockOptions[lockId];
    }

    /**
     * @dev query the current delegate for a lock
     * @param lockId the lockId set when the lock was created
     */
    function getLockDelegate(
        bytes32 lockId
    ) public view returns (address delegate) {
        return _lockDelegates[lockId];
    }

    /**
     * @dev Spend UTXOs and create new UTXOs.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param inputs array of zero or more UTXOs that the signer is authorized to spend
     * @param outputs array of zero or more new UTXOs to generate, for future transactions to spend
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
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
     *      to transfer(), but both methods can be overriden to provide different constraints.
     * @param txId a unique identifier for this transaction which must not have been used before
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
     * @dev Check that the inputs are all unspent, and remove them
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
     * @dev Check that the outputs are all new, and mark them as unspent
     */
    function _processOutputs(bytes32[] memory outputs) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (isUnspent(outputs[i]) || isLocked(outputs[i])) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _unspent[outputs[i]] = true;
        }
    }

    function _unlockHash(
        bytes32[] memory lockedInputs,
        bytes32[] memory lockedOutputs,
        bytes32[] memory outputs,
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
     * @dev Spend UTXOs and create new locked UTXOs.
     *      Locks are identified by a unique lockId, which is generated by the caller.
     *      Locked states can be spent using transferLocked() or burnLocked(), or
     *      control of the UTXOs can be delegated to another address.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param lockId unique identifier for the lock
     * @param states the input and output states (see LockStates struct)
     * @param delegate the address of the delegate who can spend the locked states (may be zero to set no delegate)
     * @param options encoded lock options (see LockOptions struct)
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {Locked} event.
     */
    function lock(
        bytes32 txId,
        bytes32 lockId,
        LockStates calldata states,
        address delegate,
        bytes calldata options,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        _lock(txId, lockId, states, delegate, options, signature, data);
    }

    /**
     * @dev Directly create new locked UTXOs.
     */
    function mintLocked(
        bytes32 txId,
        bytes32 lockId,
        bytes32[] memory lockedOutputs,
        address delegate,
        bytes calldata options,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        _lock(
            txId,
            lockId,
            LockStates({
                inputs: new bytes32[](0),
                outputs: new bytes32[](0),
                lockedOutputs: lockedOutputs
            }),
            delegate,
            options,
            signature,
            data
        );
    }

    function _lock(
        bytes32 txId,
        bytes32 lockId,
        LockStates memory states,
        address delegate,
        bytes calldata options,
        bytes calldata signature,
        bytes calldata data
    ) internal virtual {
        _processInputs(states.inputs);
        _processOutputs(states.outputs);
        _processLockedOutputs(lockId, states.lockedOutputs);

        if (options.length > 0) {
            _setLockOptions(lockId, options);
        }
        if (delegate != address(0)) {
            _lockDelegates[lockId] = delegate;
        }

        emit Locked(
            txId,
            msg.sender,
            lockId,
            states,
            delegate,
            options,
            signature,
            data
        );
    }

    /**
     * @dev Spend locked UTXOs and create new (locked or unlocked) UTXOs.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param lockId unique identifier for the lock
     * @param lockedInputs array of zero or more UTXOs locked by the given lockId
     * @param lockedOutputs array of zero or more locked UTXOs to generate, which will be tied to the lockId
     * @param outputs array of zero or more new UTXOs to generate, for future transactions to spend
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {TransferLocked} event.
     */
    function transferLocked(
        bytes32 txId,
        bytes32 lockId,
        bytes32[] memory lockedInputs,
        bytes32[] memory lockedOutputs,
        bytes32[] memory outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotaryOrDelegate(lockId) txIdNotUsed(txId) {
        _transferLocked(
            txId,
            lockId,
            lockedInputs,
            lockedOutputs,
            outputs,
            signature,
            data
        );
    }

    function _transferLocked(
        bytes32 txId,
        bytes32 lockId,
        bytes32[] memory lockedInputs,
        bytes32[] memory lockedOutputs,
        bytes32[] memory outputs,
        bytes calldata signature,
        bytes calldata data
    ) internal virtual {
        _processLockedInputs(lockId, lockedInputs);
        _processLockedOutputs(lockId, lockedOutputs);
        _processOutputs(outputs);

        LockOptions storage options = _lockOptions[lockId];
        if (options.unlockHash != 0) {
            bytes32 actualHash = _unlockHash(
                lockedInputs,
                lockedOutputs,
                outputs,
                data
            );
            if (actualHash != options.unlockHash) {
                revert NotoInvalidUnlockHash(options.unlockHash, actualHash);
            }
        }
        delete _lockOptions[lockId];

        emit TransferLocked(
            txId,
            msg.sender,
            lockId,
            lockedInputs,
            lockedOutputs,
            outputs,
            signature,
            data
        );
    }

    /**
     * @dev Update the current options for a lock.
     *      Only allowed if the lock has not been delegated.
     *
     * @param lockId unique identifier for the lock
     * @param lockedInputs array of zero or more UTXOs locked by the given lockId
     *                     (will not be modified, but will be checked for validity - allows
     *                      the call to fail if inputs have been spent/unlocked)
     * @param options encoded lock options (see LockOptions struct)
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {LockUpdated} event.
     */
    function setLockOptions(
        bytes32 txId,
        bytes32 lockId,
        bytes32[] calldata lockedInputs,
        bytes calldata options,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        if (_lockDelegates[lockId] != address(0)) {
            revert NotoAlreadyPrepared(lockId);
        }
        _checkLockedInputs(lockId, lockedInputs);
        _setLockOptions(lockId, options);
        emit LockUpdated(
            txId,
            msg.sender,
            lockId,
            lockedInputs,
            options,
            signature,
            data
        );
    }

    /**
     * @dev Pass control of a lock to a new delegate.
     *      The delegate may be set by the lock creator if no delegate has been set yet, or
     *      it may be re-delegated by the current delegate.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param lockId unique identifier for the lock
     * @param delegate the address of the delegate who can spend the locked states (may be zero to set no delegate)
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {LockDelegated} event.
     */
    function delegateLock(
        bytes32 txId,
        bytes32 lockId,
        address delegate,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotaryOrDelegate(lockId) txIdNotUsed(txId) {
        _lockDelegates[lockId] = delegate;
        emit LockDelegated(txId, msg.sender, lockId, delegate, signature, data);
    }

    function _setLockOptions(
        bytes32 lockId,
        bytes calldata options
    ) internal virtual {
        LockOptions memory lockOptions = abi.decode(options, (LockOptions));
        _lockOptions[lockId].unlockHash = lockOptions.unlockHash;
    }

    /**
     * @dev Check the inputs are all locked
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
     * @dev Check the inputs are all locked, and remove them
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
     * @dev Check the outputs are all new, and mark them as locked
     */
    function _processLockedOutputs(
        bytes32 lockId,
        bytes32[] memory outputs
    ) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (isLocked(outputs[i]) || isUnspent(outputs[i])) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _locked[outputs[i]] = lockId;
        }
    }
}
