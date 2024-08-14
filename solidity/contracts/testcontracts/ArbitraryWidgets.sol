// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

// this is nothing more than some complex type interations
contract AribtraryWidgets {

  struct Widget {
    string description;
    uint256 price;
    string[] attributes;  
  }

  struct Customer {
    address owner;
    bytes32 locator;
  }

  struct Invoice {
    Customer customer;
    Widget[] widgets;
  }

  event Invoiced(Customer customer, Widget[] widgets);

  function invoice(Invoice calldata _invoice) external payable {
    emit Invoiced(_invoice.customer, _invoice.widgets);
  }

}