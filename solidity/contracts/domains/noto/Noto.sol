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
    mapping(bytes32 => LockDetail) private _locks;
    mapping(bytes32 => address) private _approvals;

    error NotoInvalidInput(bytes32 id);
    error NotoInvalidOutput(bytes32 id);
    error NotoNotNotary(address sender);
    error NotoInvalidDelegate(bytes32 txhash, address delegate, address sender);
    error NotoInvalidLockDelegate(
        bytes32 locked,
        address delegate,
        address sender
    );
    error NotoInvalidLockOutcome(bytes32 locked, uint64 outcome);

    struct LockDetail {
        bool initialized;
        address delegate;
        mapping(uint64 => bytes32) outcomes;
        bytes data;
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
     * @dev query whether a TXO is currently in the locked list
     * @param id the UTXO identifier
     * @return unspent true or false depending on whether the identifier is in the unspent map
     */
    function isLocked(bytes32 id) public view returns (bool unspent) {
        return _locks[id].initialized;
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
     * @dev the main function of the contract, which finalizes execution of a pre-verified
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
     * @dev mint performs a transfer with no input states. Base implementation is identical
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
        bytes memory signature,
        bytes memory data
    ) internal {
        _checkInputs(inputs);
        _checkOutputs(outputs);
        emit NotoTransfer(inputs, outputs, signature, data);
    }

    /**
     * @dev Check the inputs are all existing unspent ids
     */
    function _checkInputs(bytes32[] memory inputs) internal {
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
    function _checkOutputs(bytes32[] memory outputs) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (_unspent[outputs[i]] == true) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _unspent[outputs[i]] = true;
        }
    }

    /**
     * @dev authorizes an operation to be performed by another address in a future transaction.
     *      For example, a smart contract coordinating a DVP.
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
     * @dev transfer via delegation - must be the approved delegate
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
        bytes32 txhash = _buildTXHash(inputs, outputs, data);
        if (_approvals[txhash] != msg.sender) {
            revert NotoInvalidDelegate(txhash, _approvals[txhash], msg.sender);
        }

        _transfer(inputs, outputs, signature, data);

        delete _approvals[txhash];
    }

    function _buildTXHash(
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

    /**
     * @dev Create a new locked state that can only be unlocked by a specific delegate.
     *      When the locked state is unlocked, it will be spent and replaced by exactly one new state from
     *      a set of pre-approved outcomes.
     *
     * @param locked new output state to generate, representing locked value
     * @param outcomes possible outcomes of the lock
     * @param delegate the address that is authorized to unlock the lock
     * @param signature EIP-712 signature on the original request that spawned this transaction
     * @param data any additional transaction data (opaque to the blockchain)
     */
    function createLock(
        bytes32 locked,
        LockOutcome[] calldata outcomes,
        address delegate,
        bytes calldata signature,
        bytes calldata data
    ) public virtual override onlyNotary {
        LockDetail storage stored = _locks[locked];
        for (uint256 i = 0; i < outcomes.length; i++) {
            stored.outcomes[outcomes[i].ref] = outcomes[i].state;
        }
        stored.delegate = delegate;
        stored.initialized = true;
        stored.data = data;
        emit NotoLock(locked, signature, data);
    }

    /**
     * @dev Perform a transfer and a lock simultaneously.
     */
    function transferAndLock(
        TransferParams calldata transfer_,
        LockParams calldata lock,
        bytes calldata data
    ) external virtual override onlyNotary {
        _transfer(
            transfer_.inputs,
            transfer_.outputs,
            transfer_.signature,
            data
        );
        createLock(
            lock.locked,
            lock.outcomes,
            lock.delegate,
            lock.signature,
            data
        );
    }

    /**
     * @dev Update the possible outcomes of a lock.
     *
     * @param locked locked state identifier
     * @param outcomes outcomes to create or update
     */
    function updateLock(
        bytes32 locked,
        LockOutcome[] calldata outcomes
    ) external virtual override onlyNotary {
        LockDetail storage lock = _locks[locked];
        for (uint256 i = 0; i < outcomes.length; i++) {
            lock.outcomes[outcomes[i].ref] = outcomes[i].state;
        }
    }

    /**
     * @dev Delegate ownership of the lock to a new party.
     *
     * @param locked locked state identifier
     * @param delegate the address that is authorized to unlock the lock
     */
    function delegateLock(
        bytes32 locked,
        address delegate
    ) external virtual override {
        LockDetail storage lock = _locks[locked];
        if (lock.delegate != msg.sender) {
            revert NotoInvalidLockDelegate(locked, lock.delegate, msg.sender);
        }
        lock.delegate = delegate;
    }

    /**
     * @dev Unlock a locked state and choose from one of the pre-approved outcomes.
     *
     * @param locked locked state identifier
     * @param outcome reference to the chosen lock outcome
     */
    function unlock(bytes32 locked, uint64 outcome) external virtual override {
        LockDetail storage lock = _locks[locked];
        if (lock.delegate != msg.sender) {
            revert NotoInvalidLockDelegate(locked, lock.delegate, msg.sender);
        }
        if (lock.outcomes[outcome] == 0) {
            revert NotoInvalidLockOutcome(locked, outcome);
        }

        bytes32[] memory outputs = new bytes32[](1);
        outputs[0] = lock.outcomes[outcome];
        _checkOutputs(outputs);

        emit NotoUnlock(locked, lock.outcomes[outcome], lock.data);
        delete _locks[locked];
    }
}
