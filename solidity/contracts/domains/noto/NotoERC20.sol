// SPDX-License-Identifier: Apache-2.0
// Compatible with OpenZeppelin Contracts ^5.0.0
pragma solidity ^0.8.20;

import {ERC20Upgradeable} from "@openzeppelin/contracts-upgradeable/token/ERC20/ERC20Upgradeable.sol";
import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";

/**
 * @dev Lightweight ERC20, with a fixed balance allocated to a single receiver at creation.
 */
contract NotoERC20 is Initializable, ERC20Upgradeable, OwnableUpgradeable {
    mapping(address => uint256) private _pendingDebit;
    mapping(address => uint256) private _pendingCredit;

    event UnconfirmedTransfer(
        address indexed from,
        address indexed to,
        uint256 value
    );

    error TransferPending(address account);
    error IncorrectAmount(address account, uint256 expected, uint256 actual);

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(
        string memory name,
        string memory symbol,
        address owner,
        address receiver,
        uint256 supply
    ) public initializer {
        __ERC20_init(name, symbol);
        __Ownable_init(owner);
        _mint(receiver, supply);
    }

    function redeem(address account, uint256 value) public onlyOwner {
        _burn(account, value);
    }

    function _update(
        address from,
        address to,
        uint256 value
    ) internal virtual override {
        if (from == address(0) || to == address(0)) {
            super._update(from, to, value);
            return;
        }

        if (_pendingDebit[from] > 0 || _pendingCredit[from] > 0) {
            revert TransferPending(from);
        }
        if (_pendingDebit[to] > 0 || _pendingCredit[to] > 0) {
            revert TransferPending(to);
        }
        uint256 fromBalance = balanceOf(from);
        if (fromBalance < value) {
            revert ERC20InsufficientBalance(from, fromBalance, value);
        }
        _pendingDebit[from] = value;
        _pendingCredit[to] = value;
        emit UnconfirmedTransfer(from, to, value);
    }

    function confirmTransfer(
        address from,
        address to,
        uint256 value
    ) public onlyOwner {
        if (_pendingDebit[from] != value) {
            revert IncorrectAmount(from, _pendingDebit[from], value);
        }
        if (_pendingCredit[to] != value) {
            revert IncorrectAmount(to, _pendingCredit[to], value);
        }
        _pendingDebit[from] = 0;
        _pendingCredit[to] = 0;
        super._update(from, to, value);
    }

    function pendingBalanceOf(
        address account
    ) public view virtual returns (uint256) {
        return
            balanceOf(account) +
            _pendingCredit[account] -
            _pendingDebit[account];
    }
}

contract NotoERC20Factory is Ownable {
    address public immutable logic;

    constructor() Ownable(_msgSender()) {
        logic = address(new NotoERC20());
    }

    function create(
        string memory name,
        string memory symbol,
        address receiver,
        uint256 supply
    ) public onlyOwner returns (address) {
        address instance = Clones.clone(logic);
        NotoERC20(instance).initialize(
            name,
            symbol,
            _msgSender(),
            receiver,
            supply
        );
        return instance;
    }
}
