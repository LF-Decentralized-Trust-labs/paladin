// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {IPenteExternalCall} from "../domains/interfaces/IPenteExternalCall.sol";
import {Atom, AtomFactory} from "../shared/Atom.sol";

/**
 * @title BondSubscription
 * @dev Private state between an investor and a bond custodian, tracking their subscribed units.
 */
contract BondSubscription is Ownable, IPenteExternalCall {
    address public bondAddress;
    address public custodian;
    uint256 public requestedUnits;
    address public atomFactory;

    address internal distributeBondAddress;
    bytes internal distributeBondCall;
    address internal distributePaymentAddress;
    bytes internal distributePaymentCall;

    modifier onlyCustodian() {
        require(_msgSender() == custodian, "Sender is not custodian");
        _;
    }

    constructor(
        address bondAddress_,
        uint256 units_,
        address custodian_,
        address atomFactory_
    ) Ownable(_msgSender()) {
        bondAddress = bondAddress_;
        requestedUnits = units_;
        custodian = custodian_;
        atomFactory = atomFactory_;
    }

    function preparePayment(
        address to,
        bytes calldata encodedCall
    ) external onlyOwner {
        distributePaymentAddress = to;
        distributePaymentCall = encodedCall;
    }

    function prepareBond(
        address to,
        bytes calldata encodedCall
    ) external onlyCustodian {
        distributeBondAddress = to;
        distributeBondCall = encodedCall;
    }

    function distribute() external onlyCustodian {
        require(
            distributeBondCall.length > 0,
            "Bond transfer has not been prepared"
        );
        require(
            distributePaymentCall.length > 0,
            "Payment transfer has not been prepared"
        );

        Atom.Operation[] memory operations = new Atom.Operation[](2);
        operations[0] = Atom.Operation(
            distributeBondAddress,
            distributeBondCall
        );
        operations[1] = Atom.Operation(
            distributePaymentAddress,
            distributePaymentCall
        );
        emit PenteExternalCall(
            atomFactory,
            abi.encodeCall(AtomFactory.create, operations)
        );
    }
}
