// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {INotoHooks} from "../private/interfaces/INotoHooks.sol";
import {ITransferPolicy} from "./interfaces/ITransferPolicy.sol";

/**
 * @title CSDBondTracker
 * @dev Hook logic to model a CSD bond lifecycle on top of Noto.
 */
contract CSDBondTracker is INotoHooks, ERC20, Ownable {
    enum Status {
        REQUESTED,
        REJECTED,
        READY,
        ISSUED
    }

    enum LifecycleStep {
        BOND_REQUEST,
        BOND_ISIN,
        BOND_ISSUANCE
    }
    mapping(LifecycleStep => address) internal maker;
    mapping(LifecycleStep => address) internal checker;

    Status public status;
    ITransferPolicy public transferPolicy;
    string public isin;

    constructor(
        string memory name,
        string memory symbol,
        address transferPolicy_
    ) ERC20(name, symbol) Ownable(_msgSender()) {
        status = Status.REQUESTED;
        maker[LifecycleStep.BOND_REQUEST] = _msgSender();
        transferPolicy = ITransferPolicy(transferPolicy_);
    }

    function approveRequest() external {
        require(
            checker[LifecycleStep.BOND_REQUEST] == address(0),
            "Bond request has already been approved"
        );
        require(
            _msgSender() != maker[LifecycleStep.BOND_REQUEST],
            "Maker and checker cannot be the same"
        );
        checker[LifecycleStep.BOND_REQUEST] = _msgSender();
    }

    function rejectRequest() external {
        require(
            checker[LifecycleStep.BOND_REQUEST] == address(0),
            "Bond request has already been approved"
        );
        status = Status.REJECTED;
    }

    function setISIN(string calldata isin_) external {
        require(
            checker[LifecycleStep.BOND_REQUEST] != address(0),
            "Bond request has not been checked"
        );
        require(bytes(isin).length == 0, "ISIN has already been set");
        isin = isin_;
        maker[LifecycleStep.BOND_ISIN] = _msgSender();
    }

    function approveISIN() external {
        require(
            checker[LifecycleStep.BOND_ISIN] == address(0),
            "ISIN has already been approved"
        );
        require(
            _msgSender() != maker[LifecycleStep.BOND_ISIN],
            "Maker and checker cannot be the same"
        );
        checker[LifecycleStep.BOND_ISIN] = _msgSender();
    }

    function rejectISIN() external {
        require(
            checker[LifecycleStep.BOND_ISIN] == address(0),
            "ISIN has already been approved"
        );
        isin = "";
    }

    function prepareIssuance() external {
        require(
            checker[LifecycleStep.BOND_REQUEST] != address(0),
            "Bond request has not been approved"
        );
        require(
            checker[LifecycleStep.BOND_ISIN] != address(0),
            "ISIN has not been approved"
        );
        maker[LifecycleStep.BOND_ISSUANCE] = _msgSender();
        status = Status.READY;
    }

    function onMint(
        address sender,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external onlyOwner {
        require(to == owner(), "Bond must be issued to issuer");
        require(status == Status.READY, "Bond is not ready to be issued");
        require(
            sender != maker[LifecycleStep.BOND_ISSUANCE],
            "Maker and checker cannot be the same"
        );
        checker[LifecycleStep.BOND_ISSUANCE] = sender;
        _mint(to, amount);
        status = Status.ISSUED;
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external onlyOwner {
        transferPolicy.checkTransfer(sender, from, to, amount);
        _transfer(from, to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    uint256 approvals;

    function onApproveTransfer(
        address sender,
        address from,
        address delegate,
        PreparedTransaction calldata prepared
    ) external onlyOwner {
        approvals++; // must store something on each call (see https://github.com/kaleido-io/paladin/issues/252)
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onBurn(
        address sender,
        address from,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external override {
        _burn(from, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }
}
