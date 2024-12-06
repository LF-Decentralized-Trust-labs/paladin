// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";

interface ITransferPolicy {
    function checkTransfer(
        address sender,
        address from,
        address to,
        uint256 amount
    ) external;
}
