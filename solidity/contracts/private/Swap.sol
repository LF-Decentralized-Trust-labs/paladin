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
        address indexed holder1,
        address indexed holder2
    );

    event TradePrepared(address indexed sender);
    event TradeExecuted(address indexed sender);
    event TradeCancelled(address indexed sender);

    struct UserTradeData {
        address tokenAddress;
        uint256 tokenValue;
        StateData states;
        bool prepared;
        string data;
    }

    struct TradeRequestInput {
        address holder1;
        address holder2;
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

    struct StateData {
        FullState[] inputs;
        FullState[] outputs;
    }

    struct Trade {
        address holder1;
        address holder2;
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

    function prepare(StateData calldata states) external {
        require(trade.state == State.Pending, "Trade is not pending");
        if (msg.sender == trade.holder1) {
            require(
                !trade.userTradeData1.prepared,
                "Trade has already been prepared"
            );
            trade.userTradeData1.prepared = true;
            trade.userTradeData1.states = states;
        } else if (msg.sender == trade.holder2) {
            require(
                !trade.userTradeData2.prepared,
                "Trade has already been prepared"
            );
            trade.userTradeData2.prepared = true;
            trade.userTradeData2.states = states;
        } else {
            revert("Invalid holder");
        }
        emit TradePrepared(msg.sender);
    }

    function prepared() public view returns (bool) {
        return trade.userTradeData1.prepared && trade.userTradeData2.prepared;
    }

    function execute() external {
        require(trade.state == State.Pending, "Trade is not pending");
        trade.state = State.Executed;
        emit TradeExecuted(msg.sender);
    }

    function cancel() external {
        require(trade.state == State.Pending, "Trade is not pending");
        trade.state = State.Cancelled;
        emit TradeCancelled(msg.sender);
    }
}
