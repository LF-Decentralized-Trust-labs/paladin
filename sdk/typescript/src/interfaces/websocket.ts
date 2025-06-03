import * as http from "http";
import WebSocket from "ws";
import { IEventWithData } from "./blockindex";
import { Logger } from "./logger";
import { ITransactionReceipt } from "./transaction";

export interface WebSocketSender {
  send: (json: object) => void;
  ack: (subscription: string) => void;
}

export interface WebSocketConnectCallback {
  (sender: WebSocketSender): void | Promise<void>;
}

export interface WebSocketEventCallback {
  (sender: WebSocketSender, event: WebSocketEvent): void | Promise<void>;
}

export interface WebSocketClientOptions {
  url: string;
  username?: string;
  password?: string;
  subscriptions?: string[] | WebSocketSubscription[]; // TODO: deprecate string[]
  logger?: Logger;
  heartbeatInterval?: number;
  reconnectDelay?: number;
  afterConnect?: WebSocketConnectCallback;
  socketOptions?: WebSocket.ClientOptions | http.ClientRequestArgs;
}

export interface WebSocketSubscription {
  type: "receipts" | "blockchainevents";
  name: string;
}

export interface WebSocketEvent {
  method: "ptx_subscription" | undefined;
  params: {
    subscription: string;
    result: TransactionReceiptBatch | TransactionEventBatch;
  };
}

export interface TransactionReceiptBatch {
  batchId: number;
  receipts: ITransactionReceipt[];
}

export interface TransactionEventBatch {
  batchId: number;
  events: IEventWithData[];
}
