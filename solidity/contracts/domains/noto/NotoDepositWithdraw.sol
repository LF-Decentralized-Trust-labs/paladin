// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {Noto} from "./Noto.sol";
import {NotoERC20, NotoERC20Factory} from "./NotoERC20.sol";

/**
 * @dev Noto variant that allows withdrawing Noto into an ephemeral ERC20
 *      token, then later depositing back as Noto.
 */
contract NotoDepositWithdraw is Noto {
    NotoERC20Factory internal _erc20Factory;
    mapping(bytes32 => address) internal _withdrawals;

    bytes32 private constant WITHDRAW_TYPEHASH =
        keccak256("Withdraw(address to,uint256 amount)");
    bytes32 private constant DEPOSIT_TYPEHASH =
        keccak256("Deposit(address from,uint256 amount)");

    error NotoNoInputs();
    error NotoNoOutputs();
    error NotoInvalidSigner(address expected, address actual);

    function initialize(
        address notaryAddress,
        bytes calldata data
    ) public virtual override initializer returns (bytes memory) {
        _erc20Factory = new NotoERC20Factory();
        return super.initialize(notaryAddress, data);
    }

    /**
     * @dev Withdraw some Noto UTXOs into a new ERC20 contract.
     *
     * @param inputs Array of zero or more outputs of a previous function call against this
     *      contract that have not yet been spent, and the signer is authorized to spend
     * @param outputs Array of zero or more new outputs to generate, for future transactions to spend
     * @param withdrawalOutputs Array of one or more new outputs that will track the ERC20 token balance
     * @param to Address to receive the new ERC20 tokens
     * @param amount Amount of ERC20 tokens to create
     * @param data Any additional transaction data (opaque to the blockchain)
     */
    function withdraw(
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes32[] memory withdrawalOutputs,
        address to,
        uint256 amount,
        bytes memory transferSignature,
        bytes memory withdrawSignature,
        bytes memory data
    ) external onlyNotary {
        if (withdrawalOutputs.length == 0) {
            revert NotoNoOutputs();
        }

        _transfer(inputs, outputs, transferSignature, data);

        bytes32 hash = _hashTypedDataV4(
            keccak256(abi.encode(WITHDRAW_TYPEHASH, to, amount))
        );
        address signer = ECDSA.recover(hash, withdrawSignature);
        if (signer != to) {
            revert NotoInvalidSigner(to, signer);
        }

        address erc20 = _erc20Factory.create("", "", to, amount);
        _checkWithdrawalOutputs(withdrawalOutputs, erc20);
    }

    /**
     * @dev Redeem ERC20 tokens from a previous withdrawal, creating new Noto UTXOs.
     *
     * @param withdrawalInputs Array of zero or more outputs of a previous withdrawal on this
     *      contract that have not yet been spent, and the signer is authorized to spend
     * @param withdrawalOutputs Array of one or more new outputs that will track the remaining
     *      ERC20 token balance
     * @param outputs Array of zero or more new outputs to generate, for future transactions to spend
     * @param from Address from which to redeem ERC20 tokens
     * @param amount Amount of ERC20 tokens to redeem
     * @param data Any additional transaction data (opaque to the blockchain)
     */
    function deposit(
        bytes32[] memory withdrawalInputs,
        bytes32[] memory withdrawalOutputs,
        bytes32[] memory outputs,
        address from,
        uint256 amount,
        bytes memory transferSignature,
        bytes memory depositSignature,
        bytes memory data
    ) external onlyNotary {
        if (withdrawalInputs.length == 0) {
            revert NotoNoInputs();
        }
        address erc20 = _withdrawals[withdrawalInputs[0]];
        if (erc20 == address(0)) {
            revert NotoInvalidInput(withdrawalInputs[0]);
        }

        bytes32[] memory inputs;
        _transfer(inputs, outputs, transferSignature, data);

        bytes32 hash = _hashTypedDataV4(
            keccak256(abi.encode(DEPOSIT_TYPEHASH, from, amount))
        );
        address signer = ECDSA.recover(hash, depositSignature);
        if (signer != from) {
            revert NotoInvalidSigner(from, signer);
        }

        _checkWithdrawalInputs(withdrawalInputs, erc20);
        _checkWithdrawalOutputs(withdrawalOutputs, erc20);
        NotoERC20(erc20).redeem(from, amount);
    }

    function _checkWithdrawalInputs(
        bytes32[] memory inputs,
        address erc20
    ) internal {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (_withdrawals[inputs[i]] != erc20) {
                revert NotoInvalidInput(inputs[i]);
            }
            delete _withdrawals[inputs[i]];
        }
    }

    function _checkWithdrawalOutputs(
        bytes32[] memory outputs,
        address erc20
    ) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (_withdrawals[outputs[i]] != address(0)) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _withdrawals[outputs[i]] = erc20;
        }
    }
}
