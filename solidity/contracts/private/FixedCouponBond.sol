// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/access/Ownable.sol";

/**
 * @title FixedCouponBond
 * @dev Simple bond with fixed coupon payments.
 *
 * This contract tracks the current status of a bond, and should be atomically linked
 * with token actions on another contract to distribute coupon payments.
 */
contract FixedCouponBond is Ownable {
    // For simplicity, a year is exactly 365 days.
    uint256 public termUnit = 365 days;

    enum Status {
        UNINITIALIZED,
        ISSUED,
        REDEEMED
    }

    struct BondDetails {
        uint256 issueDate; // issuance date (in epoch seconds)
        uint8 termLength; // term length (using term unit above)
        uint256 faceValue; // face value (whole integer, no decimals)
        address couponToken; // address of token for coupon payment
        uint256 couponRate; // coupon rate (percentage per term unit)
        uint8 couponRateDecimals; // number of decimals in coupon rate
        uint8 numPayments; // total number of coupon payments
    }

    address public issuer;
    BondDetails public details;
    uint256 public quantity;
    uint8 public claimedPayments;
    Status public status;

    mapping(address => bool) issuerApprovals;
    mapping(address => bool) ownerApprovals;

    modifier onlyIssuerOrApproved() {
        require(
            _msgSender() == issuer || issuerApprovals[_msgSender()],
            "Sender is not issuer or approved"
        );
        _;
    }

    modifier onlyOwnerOrApproved() {
        require(
            _msgSender() == owner() || ownerApprovals[_msgSender()],
            "Sender is not owner or approved"
        );
        _;
    }

    constructor(
        address recipient_,
        BondDetails memory details_
    ) Ownable(recipient_) {
        issuer = _msgSender();
        details = details_;
        status = Status.UNINITIALIZED;
    }

    /**
     * Issue the bond.
     */
    function issue(uint256 quantity_) external onlyIssuerOrApproved {
        require(status == Status.UNINITIALIZED, "Bond is already issued");
        quantity = quantity_;
        status = Status.ISSUED;
    }

    /**
     * Initialize a bond that was transferred from a previous owner.
     */
    function recordTransfer(
        uint256 quantity_,
        uint8 claimedPayments_
    ) external onlyIssuerOrApproved {
        require(status == Status.UNINITIALIZED, "Bond is already issued");
        quantity = quantity_;
        claimedPayments = claimedPayments_;
        status = Status.ISSUED;
    }

    /**
     * Get the total amount that will be paid out in coupons over the bond's lifetime.
     */
    function couponTotal() public view returns (uint256) {
        return
            (quantity *
                details.faceValue *
                details.couponRate *
                details.termLength) / (10 ** details.couponRateDecimals * 100);
    }

    /**
     * Get the amount of each coupon payment.
     */
    function couponPayment() public view returns (uint256) {
        return couponTotal() / details.numPayments;
    }

    /**
     * Get the maturity date.
     */
    function maturityDate() public view returns (uint256) {
        return details.issueDate + (details.termLength * termUnit);
    }

    /**
     * Get the number of payments that have elapsed.
     */
    function elapsedPayments() public view returns (uint256) {
        if (block.timestamp == details.issueDate) {
            return 0;
        } else if (block.timestamp >= maturityDate()) {
            return details.numPayments;
        }
        uint256 paymentInterval = (details.termLength * termUnit) /
            details.numPayments;
        uint256 elapsedTime = block.timestamp - details.issueDate;
        return elapsedTime / paymentInterval;
    }

    /**
     * Get the number of payments available to claim.
     */
    function availablePayments() public view returns (uint256) {
        return elapsedPayments() - claimedPayments;
    }

    /**
     * Get the amount of interest accrued by coupons to date.
     */
    function earnedInterest() external view returns (uint256) {
        uint256 payments = elapsedPayments();
        if (payments == 0) {
            return 0;
        } else if (payments == details.numPayments) {
            return couponTotal();
        } else {
            return couponPayment() * payments;
        }
    }

    /**
     * Get the amount of interest claimed to date.
     */
    function claimedInterest() external view returns (uint256) {
        if (claimedPayments == 0) {
            return 0;
        } else if (claimedPayments == details.numPayments) {
            return couponTotal();
        } else {
            return couponPayment() * claimedPayments;
        }
    }

    function approve(address delegate, bool approved) external {
        if (_msgSender() == owner()) {
            ownerApprovals[delegate] = approved;
        } else if (_msgSender() == issuer) {
            issuerApprovals[delegate] = approved;
        } else {
            revert("Sender is not owner or issuer");
        }
    }

    function claimPayment() external onlyOwnerOrApproved {
        require(status == Status.ISSUED, "Bond is not active");
        require(
            claimedPayments < elapsedPayments(),
            "No payments available to claim"
        );
        claimedPayments++;
    }

    function transfer(uint256 amount) external onlyOwnerOrApproved {
        require(status == Status.ISSUED, "Bond is not active");
        require(amount >= quantity, "Insufficient balance");
        require(availablePayments() == 0, "Cannot transfer with unclaimed payments");
        quantity -= amount;
    }

    function redeem() external onlyOwnerOrApproved {
        require(status == Status.ISSUED, "Bond is not active");
        status = Status.REDEEMED;
    }
}
