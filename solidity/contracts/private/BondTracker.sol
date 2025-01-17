// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {NotoTrackerERC20} from "./NotoTrackerERC20.sol";
import {InvestorList} from "./InvestorList.sol";

/**
 * @title BondTracker
 * @dev Hook logic to model a simple bond lifecycle on top of Noto.
 */
contract BondTracker is NotoTrackerERC20 {
    enum Status {
        INITIALIZED,
        ISSUED,
        DISTRIBUTION_STARTED,
        DISTRIBUTION_CLOSED,
        ACTIVE
    }

    Status internal _status;
    address internal _publicTracker;
    address internal _issuer;
    address internal _custodian;
    InvestorList public investorList;

    modifier onlyIssuer(address sender) {
        require(sender == _issuer, "Sender is not bond issuer");
        _;
    }

    modifier onlyCustodian(address sender) {
        require(sender == _custodian, "Sender is not bond custodian");
        _;
    }

    constructor(
        string memory name,
        string memory symbol,
        address custodian,
        address publicTracker
    ) NotoTrackerERC20(name, symbol) {
        _status = Status.INITIALIZED;
        _publicTracker = publicTracker;
        _issuer = msg.sender;
        _custodian = custodian;
        investorList = new InvestorList(custodian);
    }

    function beginDistribution(
        uint256 discountPrice,
        uint256 minimumDenomination
    ) external onlyCustodian(msg.sender) {
        require(_status == Status.ISSUED, "Bond has not been issued");
        _status = Status.DISTRIBUTION_STARTED;
        emit PenteExternalCall(
            _publicTracker,
            abi.encodeWithSignature(
                "beginDistribution(uint256,uint256)",
                discountPrice,
                minimumDenomination
            )
        );
    }

    function closeDistribution() external onlyCustodian(msg.sender) {
        _status = Status.DISTRIBUTION_CLOSED;
        emit PenteExternalCall(
            _publicTracker,
            abi.encodeWithSignature("closeDistribution()")
        );
    }

    function setActive() external onlyIssuer(msg.sender) {
        require(
            _status == Status.DISTRIBUTION_CLOSED,
            "Bond is not ready to be activated"
        );
        _status = Status.ACTIVE;
        emit PenteExternalCall(
            _publicTracker,
            abi.encodeWithSignature("setActive()")
        );
    }

    function _checkTransfer(
        address sender,
        address from,
        address to,
        uint256 amount
    ) internal view {
        investorList.checkTransfer(sender, from, to, amount);
    }

    function _checkTransfers(
        address sender,
        address from,
        UnlockRecipient[] calldata recipients
    ) internal view {
        for (uint256 i = 0; i < recipients.length; i++) {
            investorList.checkTransfer(
                sender,
                from,
                recipients[i].to,
                recipients[i].amount
            );
        }
    }

    function onMint(
        address sender,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyIssuer(sender) {
        require(to == _custodian, "Bond must be issued to custodian");
        require(_status == Status.INITIALIZED, "Bond has already been issued");

        _onMint(sender, to, amount, data, prepared);
        _status = Status.ISSUED;

        emit PenteExternalCall(
            _publicTracker,
            abi.encodeWithSignature("onIssue(address,uint256)", to, amount)
        );
    }

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlySelf(sender, from) {
        _checkTransfer(sender, from, to, amount);
        _onTransfer(sender, from, to, amount, data, prepared);

        if (_status == Status.DISTRIBUTION_STARTED && from == _custodian) {
            emit PenteExternalCall(
                _publicTracker,
                abi.encodeWithSignature("onDistribute(uint256)", amount)
            );
        }
    }

    function onUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyLockOwner(sender, lockId) {
        _checkTransfers(sender, _locks.ownerOf(lockId), recipients);
        _onUnlock(sender, lockId, recipients, data, prepared);
    }

    function onPrepareUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyLockOwner(sender, lockId) {
        _checkTransfers(sender, _locks.ownerOf(lockId), recipients);
        _onPrepareUnlock(sender, lockId, recipients, data, prepared);
    }
}
