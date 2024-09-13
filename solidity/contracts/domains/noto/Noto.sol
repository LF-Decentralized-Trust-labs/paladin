// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {EIP712Upgradeable} from "@openzeppelin/contracts-upgradeable/utils/cryptography/EIP712Upgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {INoto} from "../interfaces/INoto.sol";
import {IPaladinContract_V0} from "../interfaces/IPaladinContract.sol";

/// @title A sample on-chain implementation of a Confidential UTXO (C-UTXO) pattern,
///        with participant confidentiality and anonymity based on notary submission and
///        validation of transactions.
/// @author Kaleido, Inc.
/// @dev Transaction pre-verification is performed by a notary.
///      The EVM ledger provides double-spend protection on the transaction outputs,
///      and provides a deterministic linkage (a DAG) of inputs and outputs.
///
///      The notary must authorize every transaction by either:
///
///      1. Submitting the transaction directly
///
///      2. Pre-authorizing another EVM address to perform a transaction, by storing
///         the EIP-712 typed-data hash of the transaction in an approval record.
///         This allows coordination of DVP with other smart contracts, which could
///         be using any model programmable via EVM (not just C-UTXO)
///
contract Noto is
    EIP712Upgradeable,
    UUPSUpgradeable,
    INoto,
    IPaladinContract_V0
{
    mapping(bytes32 => bool) private _unspent;
    mapping(bytes32 => ApprovalRecord) private _approvals;
    address _notary;

    error NotoInvalidNotary(address signer, address notary);
    error NotoInvalidInput(bytes32 id);
    error NotoInvalidOutput(bytes32 id);
    error NotoNotNotary(address sender);
    error NotoInvalidDelegate(bytes32 txhash, address delegate, address sender);
    error NotoUnsupportedConfigType(bytes4 configSelector);

    // Config follows the convention of a 4 byte type selector, followed by ABI encoded bytes
    bytes4 public constant NotoConfigID_V0 = 0x00010000;

    struct NotoConfig_V0 {
        string notaryLookup;
        address notaryAddress;
        bytes32 variant;
    }

    bytes32 public constant NotoVariantDefault =
        0x0000000000000000000000000000000000000000000000000000000000000000;

    bytes32 private constant TRANSFER_TYPEHASH =
        keccak256("Transfer(bytes32[] inputs,bytes32[] outputs,bytes data)");

    struct ApprovalRecord {
        address delegate;
    }

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
        bytes32 transactionId,
        address domain,
        address notary,
        bytes calldata config
    ) public virtual initializer {
        __EIP712_init("noto", "0.0.1");
        _notary = notary;

        NotoConfig_V0 memory configOut = _decodeConfig(config);
        configOut.notaryAddress = notary;
        configOut.variant = NotoVariantDefault;

        emit PaladinNewSmartContract_V0(
            transactionId,
            domain,
            _encodeConfig(configOut)
        );
    }

    function _decodeConfig(
        bytes calldata config
    ) internal pure returns (NotoConfig_V0 memory) {
        bytes4 configSelector = bytes4(config[0:4]);
        if (configSelector == NotoConfigID_V0) {
            NotoConfig_V0 memory configOut;
            (configOut.notaryLookup) = abi.decode(config[4:], (string));
            return configOut;
        }
        revert NotoUnsupportedConfigType(configSelector);
    }

    function _encodeConfig(
        NotoConfig_V0 memory config
    ) internal pure returns (bytes memory) {
        bytes memory configOut = abi.encode(
            config.notaryLookup,
            config.notaryAddress,
            config.variant
        );
        return bytes.concat(NotoConfigID_V0, configOut);
    }

    function _authorizeUpgrade(address) internal override onlyNotary {}

    /// @dev query whether a TXO is currently in the unspent list
    /// @param id the UTXO identifier
    /// @return unspent true or false depending on whether the identifier is in the unspent map
    function isUnspent(bytes32 id) public view returns (bool unspent) {
        return _unspent[id];
    }

    /// @dev query whether an approval exists for the given transaction
    /// @param txhash the transaction hash
    /// @return delegate the non-zero owner address, or zero if the TXO ID is not in the approval map
    function getApproval(
        bytes32 txhash
    ) public view returns (address delegate) {
        return _approvals[txhash].delegate;
    }

    /**
     * @dev authorizes a operation to be performed by another address in a future transaction.
     *      For example, a smart contract coordinating a DVP.
     *
     *      Note the txhash will only be spendable if it is exactly correct for
     *      the inputs/outputs/data that are later supplied in useDelegation.
     *      This approach is gas-efficient as it means:
     *      - The inputs/outputs/data are not stored on-chain at any point
     *      - The EIP-712 hash is only calculated on-chain once, in approvedTranfer()
     *
     * @param delegate the address that is authorized to submit the transaction
     * @param txhash the pre-calculated hash of the transaction that is delegated
     *
     * Emits a {UTXOApproved} event.
     */
    function _approve(
        address delegate,
        bytes32 txhash,
        bytes calldata signature
    ) internal {
        _approvals[txhash].delegate = delegate;
        emit UTXOApproved(delegate, txhash, signature);
    }

    /**
     * @dev transfer via delegation - must be the approved delegate
     *
     * @param inputs as per transfer()
     * @param outputs as per transfer()
     * @param data as per transfer()
     *
     * Emits a {UTXOTransfer} event.
     */
    function approvedTransfer(
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) public {
        bytes32 txhash = _buildTXHash(inputs, outputs, data);
        if (_approvals[txhash].delegate != msg.sender) {
            revert NotoInvalidDelegate(
                txhash,
                _approvals[txhash].delegate,
                msg.sender
            );
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
     * @dev the main function of the contract, which finalizes execution of a pre-verified
     *      transaction. The inputs and outputs are all opaque to this on-chain function.
     *      Provides ordering and double-spend protection.
     *
     * @param inputs Array of zero or more outputs of a previous function call against this
     *      contract that have not yet been spent, and the signer is authorized to spend.
     * @param outputs Array of zero or more new outputs to generate, for future transactions to spend.
     * @param data Any additional transaction data (opaque to the blockchain)
     *
     * Emits a {UTXOTransfer} event.
     */
    function _transfer(
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes memory signature,
        bytes memory data
    ) internal {
        // Check the inputs are all existing unspent ids
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (_unspent[inputs[i]] == false) {
                revert NotoInvalidInput(inputs[i]);
            }
            delete (_unspent[inputs[i]]);
        }

        // Check the outputs are all new unspent ids
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (_unspent[outputs[i]] == true) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _unspent[outputs[i]] = true;
        }

        emit UTXOTransfer(inputs, outputs, signature, data);
    }

    function mint(
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary {
        bytes32[] memory inputs;
        _transfer(inputs, outputs, signature, data);
    }

    function transfer(
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary {
        _transfer(inputs, outputs, signature, data);
    }

    function approve(
        address delegate,
        bytes32 txhash,
        bytes calldata signature
    ) external virtual onlyNotary {
        _approve(delegate, txhash, signature);
    }
}
