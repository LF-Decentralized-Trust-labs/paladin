import * as http from "http";
import WebSocket from "ws";
import { ITransactionReceipt } from "./transaction";
import { Logger } from "./logger";

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
  subscriptions?: string[];
  username?: string;
  password?: string;
  logger?: Logger;
  heartbeatInterval?: number;
  reconnectDelay?: number;
  afterConnect?: WebSocketConnectCallback;
  socketOptions?: WebSocket.ClientOptions | http.ClientRequestArgs;
}

export interface WebSocketEvent {
  method: "ptx_subscription" | undefined;
  params: {
    subscription: string;
    result: {
      receipts: ITransactionReceipt[];
    };
  };
}
