// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC20Burnable.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "../domains/interfaces/ILockableCapability.sol";

/**
 * @title ERC20Lockable
 * @dev Simple ERC20 token that implements ILockableCapability for balance locking.
 *      Locks represent reserved token amounts that can be spent or cancelled later.
 *      Locked balances are tracked as a subset of the owner's total balance.
 */
contract ERC20Lockable is ERC20, ERC20Burnable, Ownable, ILockableCapability {
    struct CreateLockInputs {
        uint256 amount;
        address recipient;
    }

    struct TokenLock {
        address owner;
        address spender;
        CreateLockInputs content;
        bytes32 spendCommitment;
        bytes32 cancelCommitment;
        bool active;
    }

    // ERC-8074 type selector for the bytes returned by getLockContent.
    // bytes4(keccak256("ERC20LockContent(uint256 amount,address recipient)"))
    string public constant ERC20LockContentType =
        "ERC20LockContent(uint256 amount,address recipient)";
    bytes4 public constant ERC20LockContentSelector =
        bytes4(keccak256(bytes(ERC20LockContentType)));

    mapping(bytes32 => TokenLock) private _locks;
    mapping(address => uint256) private _lockedBalances;
    uint256 private _lockCounter;

    error InvalidAmount(uint256 amount);

    constructor(
        string memory name,
        string memory symbol
    ) ERC20(name, symbol) Ownable(_msgSender()) {}

    function mint(address to, uint256 amount) public onlyOwner {
        _mint(to, amount);
    }

    /**
     * @dev Override transfer to check unlocked balance
     */
    function _update(
        address from,
        address to,
        uint256 value
    ) internal override {
        if (from != address(0)) {
            uint256 unlockedBalance = balanceOf(from) - _lockedBalances[from];
            if (value > unlockedBalance) {
                revert ERC20InsufficientBalance(from, unlockedBalance, value);
            }
        }
        super._update(from, to, value);
    }

    /**
     * @dev Get the locked balance for an account
     */
    function lockedBalanceOf(address account) external view returns (uint256) {
        return _lockedBalances[account];
    }

    /**
     * @dev Get the unlocked (available) balance for an account
     */
    function unlockedBalanceOf(
        address account
    ) external view returns (uint256) {
        return balanceOf(account) - _lockedBalances[account];
    }

    /**
     * @dev Create a lock for a token amount.
     */
    function createLock(
        bytes calldata createInputs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external returns (bytes32 lockId) {
        CreateLockInputs memory inputs = abi.decode(createInputs, (CreateLockInputs));
        require(inputs.amount > 0, InvalidAmount(inputs.amount));

        uint256 unlockedBalance = balanceOf(msg.sender) -
            _lockedBalances[msg.sender];
        require(
            inputs.amount <= unlockedBalance,
            ERC20InsufficientBalance(msg.sender, unlockedBalance, inputs.amount)
        );

        lockId = keccak256(
            abi.encode(address(this), msg.sender, ++_lockCounter)
        );

        _locks[lockId] = TokenLock({
            owner: msg.sender,
            spender: msg.sender,
            content: inputs,
            spendCommitment: spendCommitment,
            cancelCommitment: cancelCommitment,
            active: true
        });

        _lockedBalances[msg.sender] += inputs.amount;

        emit LockCreated(
            lockId,
            msg.sender,
            msg.sender,
            spendCommitment,
            cancelCommitment,
            data
        );
    }

    /**
     * @dev Update an existing lock (only if not delegated)
     */
    function updateLock(
        bytes32 lockId,
        bytes calldata /* updateInputs */,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );
        require(lock.spender == lock.owner, LockImmutable(lockId));

        lock.spendCommitment = spendCommitment;
        lock.cancelCommitment = cancelCommitment;

        emit LockUpdated(
            lockId,
            lock.owner,
            spendCommitment,
            cancelCommitment,
            data
        );
    }

    /**
     * @dev Delegate spending authority to another address
     */
    function delegateLock(
        bytes32 lockId,
        bytes calldata /* delegateInputs */,
        address newSpender,
        bytes calldata data
    ) external {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );

        address oldSpender = lock.spender;
        lock.spender = newSpender;

        emit LockDelegated(lockId, oldSpender, newSpender, data);
    }

    /**
     * @dev Spend the locked tokens (transfer to recipient)
     */
    function spendLock(
        bytes32 lockId,
        bytes calldata /* spendInputs */,
        bytes calldata data
    ) external {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );

        uint256 amount = lock.content.amount;
        address owner = lock.owner;
        address recipient = lock.content.recipient;

        _lockedBalances[owner] -= amount;
        delete _locks[lockId];

        _transfer(owner, recipient, amount);

        emit LockSpent(lockId, msg.sender, data);
    }

    /**
     * @dev Cancel the lock (unlock the tokens)
     */
    function cancelLock(
        bytes32 lockId,
        bytes calldata /* cancelInputs */,
        bytes calldata data
    ) external {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );

        _lockedBalances[lock.owner] -= lock.content.amount;
        delete _locks[lockId];

        emit LockCancelled(lockId, msg.sender, data);
    }

    /**
     * @dev Get lock information
     */
    function getLock(bytes32 lockId) external view returns (LockInfo memory) {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        return
            LockInfo({
                owner: lock.owner,
                spender: lock.spender,
                spendCommitment: lock.spendCommitment,
                cancelCommitment: lock.cancelCommitment
            });
    }

    /**
     * @dev Get the implementation-specific lock content (amount and recipient)
     */
    function getLockContent(
        bytes32 lockId
    ) external view returns (bytes memory) {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        return bytes.concat(ERC20LockContentSelector, abi.encode(lock.content));
    }

    /**
     * @dev Query whether a lock is currently active
     */
    function isLockActive(bytes32 lockId) external view returns (bool) {
        return _locks[lockId].active;
    }
}
