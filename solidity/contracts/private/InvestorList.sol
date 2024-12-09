// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {ITransferPolicy} from "./interfaces/ITransferPolicy.sol";

/**
 * @title InvestorList
 * @dev Simple allow/deny list for tracking registered investors.
 */
contract InvestorList is Ownable, ITransferPolicy {
    event InvestorAdded(address indexed investor);
    event InvestorRemoved(address indexed investor);

    address[] public investors;
    mapping(address => uint256) internal _investorIndex;

    constructor(address initialOwner) Ownable(initialOwner) {}

    function addInvestor(address addr) public onlyOwner {
        require(_investorIndex[addr] == 0, "Investor already added");
        investors.push(addr);
        _investorIndex[addr] = investors.length;
        emit InvestorAdded(addr);
    }

    function addInvestors(address[] calldata addresses) external onlyOwner {
        for (uint256 i = 0; i < addresses.length; i++) {
            addInvestor(addresses[i]);
        }
    }

    function removeInvestor(address addr) external onlyOwner {
        uint256 idx = _investorIndex[addr];
        require(idx != 0, "Investor already removed");
        delete _investorIndex[addr];
        delete investors[idx - 1];
        emit InvestorRemoved(addr);
    }

    function isRegistered(address addr) public view returns (bool) {
        return _investorIndex[addr] != 0;
    }

    function listInvestors() public view returns (address[] memory) {
        address[] memory _investors = new address[](investors.length);
        for (uint256 i = 0; i < investors.length; i++) {
            _investors[i] = investors[i];
        }

        return _investors;
    }

    function checkTransfer(
        address sender,
        address from,
        address to,
        uint256 amount
    ) external view {
        require(isRegistered(to), "Investor is not registered");
    }
}
