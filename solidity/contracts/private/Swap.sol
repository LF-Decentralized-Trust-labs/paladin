// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title Swap
 * @dev Tracks the state of a proposed token swap, while the actual swap happens on another contract.
 */
contract Swap {
    enum State {
        Pending,
        Executed,
        Cancelled
    }

    event TradeRequested(
        address indexed operator,
        string holder1,
        string holder2
    );

    event TradePrepared(address indexed sender, string holder);
    event TradeAccepted(address indexed sender);
    event TradeExecuted(address indexed sender);
    event TradeCancelled(address indexed sender);

    struct UserTradeData {
        address sender;
        address tokenAddress;
        uint256 tokenValue;
        PreparedData prepared;
        bool accepted;
        string data;
    }

    struct TradeRequestInput {
        string holder1;
        string holder2;
        address tokenAddress1;
        address tokenAddress2;
        uint256 tokenValue1;
        uint256 tokenValue2;
        string tradeData1;
        string tradeData2;
    }

    struct FullState {
        bytes32 id;
        bytes32 schema;
        bytes data;
    }

    struct PreparedData {
        FullState[] inputs;
        FullState[] outputs;
    }

    struct Trade {
        string holder1;
        string holder2;
        UserTradeData userTradeData1;
        UserTradeData userTradeData2;
        State state;
    }

    Trade public trade;

    constructor(TradeRequestInput memory inputData) {
        trade.holder1 = inputData.holder1;
        trade.holder2 = inputData.holder2;
        trade.userTradeData1.tokenAddress = inputData.tokenAddress1;
        trade.userTradeData1.tokenValue = inputData.tokenValue1;
        trade.userTradeData1.data = inputData.tradeData1;
        trade.userTradeData2.tokenAddress = inputData.tokenAddress2;
        trade.userTradeData2.tokenValue = inputData.tokenValue2;
        trade.userTradeData2.data = inputData.tradeData2;
        trade.state = State.Pending;
    }

    function stringEqual(
        string memory s1,
        string memory s2
    ) internal pure returns (bool) {
        return
            keccak256(abi.encodePacked(s1)) == keccak256(abi.encodePacked(s2));
    }

    function prepare(
        string calldata holder,
        PreparedData calldata prepared
    ) external {
        require(trade.state == State.Pending, "Trade is not pending");
        if (stringEqual(holder, trade.holder1)) {
            require(
                trade.userTradeData1.sender == address(0),
                "Trade has already been prepared"
            );
            trade.userTradeData1.sender = msg.sender;
            trade.userTradeData1.prepared = prepared;
        } else if (stringEqual(holder, trade.holder2)) {
            require(
                trade.userTradeData2.sender == address(0),
                "Trade has already been prepared"
            );
            trade.userTradeData2.sender = msg.sender;
            trade.userTradeData2.prepared = prepared;
        } else {
            revert("Invalid holder");
        }
        emit TradePrepared(msg.sender, holder);
    }

    function prepared() public view returns (bool) {
        return
            trade.userTradeData1.sender != address(0) &&
            trade.userTradeData2.sender != address(0);
    }

    function accept() external {
        require(trade.state == State.Pending, "Trade is not pending");
        require(
            prepared(),
            "Trade has not been prepared by all token holders yet"
        );
        if (msg.sender == trade.userTradeData1.sender) {
            require(
                !trade.userTradeData1.accepted,
                "Trade has already been accepted"
            );
            trade.userTradeData1.accepted = true;
        } else if (msg.sender == trade.userTradeData2.sender) {
            require(
                !trade.userTradeData2.accepted,
                "Trade has already been accepted"
            );
            trade.userTradeData2.accepted = true;
        } else {
            revert("Invalid sender");
        }
        emit TradeAccepted(msg.sender);
    }

    function accepted() public view returns (bool) {
        return trade.userTradeData1.accepted && trade.userTradeData2.accepted;
    }

    function execute() external {
        require(trade.state == State.Pending, "Trade is not pending");
        require(
            accepted(),
            "Trade has not been accepted by all token holders yet"
        );
        trade.state = State.Executed;
        emit TradeExecuted(msg.sender);
    }

    function cancel() external {
        require(trade.state == State.Pending, "Trade is not pending");
        trade.state = State.Cancelled;
        emit TradeCancelled(msg.sender);
    }
}
