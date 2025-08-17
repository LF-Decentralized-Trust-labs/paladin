import * as http from "http";
import WebSocket from "ws";
import { IEventWithData } from "./blockindex";
import { Logger } from "./logger";
import { ITransactionReceipt } from "./transaction";
import { IPrivacyGroupMessage } from "./privacygroups";

export interface WebSocketSender {
  send: (json: object) => void;
  ack: (subscription: string) => void;
}

export interface WebSocketConnectCallback {
  (sender: WebSocketSender): void | Promise<void>;
}

export interface WebSocketEventCallback<TEvent> {
  (sender: WebSocketSender, event: TEvent): void | Promise<void>;
}

export interface WebSocketClientOptions<TMessageTypes extends string> {
  url: string;
  username?: string;
  password?: string;
  subscriptions?: WebSocketSubscription<TMessageTypes>[];
  logger?: Logger;
  heartbeatInterval?: number;
  reconnectDelay?: number;
  afterConnect?: WebSocketConnectCallback;
  socketOptions?: WebSocket.ClientOptions | http.ClientRequestArgs;
}

export interface WebSocketSubscription<TMessageTypes extends string> {
  type: TMessageTypes;
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

export interface PrivacyGroupWebSocketEvent {
  method: "pgroup_subscription" | undefined;
  params: {
    subscription: string;
    result: IPrivacyGroupMessageBatch;
  };
}

export interface IPrivacyGroupMessageBatch {
  batchId: number;
  messages: IPrivacyGroupMessage[];
}
