// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {TokenDistribution} from "./TokenDistribution.sol";

/**
 * @title BondTrackerPublic
 * @dev Tracking for publically-available information about a bond. Designed to pair
 *      with a BondTracker contract deployed in a privacy group.
 */
contract BondTrackerPublic is Ownable {
    enum Status {
        INITIALIZED,
        ISSUED,
        DISTRIBUTION_STARTED,
        DISTRIBUTION_COMPLETE,
        DISTRIBUTION_CLOSED,
        ACTIVE
    }

    event StatusChanged(Status newStatus);
    event CustodianChanged(address indexed newCustodian);
    event BondDetailsChanged(
        uint256 indexed issueDate,
        uint256 indexed maturityDate,
        uint256 indexed faceValue,
        address currencyToken
    );

    address public custodian;
    TokenDistribution public distribution;
    address public currencyToken;
    uint256 public totalSupply;

    Status internal _status;
    uint256 internal _issueDate;
    uint256 internal _maturityDate;
    uint256 internal _faceValue;

    constructor(
        address owner,
        uint256 issueDate_,
        uint256 maturityDate_,
        address currencyToken_,
        uint256 faceValue_
    ) Ownable(owner) {
        require(
            maturityDate_ > issueDate_,
            "Maturity date must be after issue date"
        );
        currencyToken = currencyToken_;
        _status = Status.INITIALIZED;
        _issueDate = issueDate_;
        _maturityDate = maturityDate_;
        _faceValue = faceValue_;
        emit StatusChanged(_status);
        emit BondDetailsChanged(
            issueDate_,
            maturityDate_,
            faceValue_,
            currencyToken
        );
    }

    function onIssue(address to, uint256 amount) external onlyOwner {
        custodian = to;
        totalSupply = amount;
        _status = Status.ISSUED;
        emit StatusChanged(_status);
        emit CustodianChanged(to);
    }

    function beginDistribution(
        uint256 discountPrice,
        uint256 minimumDenomination
    ) external onlyOwner {
        require(
            address(distribution) == address(0),
            "Distribution has already been initialized"
        );
        distribution = new TokenDistribution(
            address(this),
            totalSupply,
            currencyToken,
            discountPrice,
            minimumDenomination
        );
        _status = Status.DISTRIBUTION_STARTED;
        emit StatusChanged(_status);
    }

    function onDistribute(uint256 amount) external onlyOwner {
        require(
            address(distribution) != address(0),
            "Distribution has not been initialized"
        );
        distribution.purchase(amount);
        if (distribution.availableUnits() == 0) {
            _status = Status.DISTRIBUTION_COMPLETE;
            emit StatusChanged(_status);
        }
    }

    function closeDistribution() external onlyOwner {
        require(
            _status == Status.DISTRIBUTION_COMPLETE,
            "Distribution is not complete"
        );
        _status = Status.DISTRIBUTION_CLOSED;
        emit StatusChanged(_status);
    }

    function setActive() external onlyOwner {
        require(
            _status == Status.DISTRIBUTION_CLOSED,
            "Bond is not ready to be activated"
        );
        _status = Status.ACTIVE;
        emit StatusChanged(_status);
    }
}
