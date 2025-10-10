import { IncomingMessage } from "http";
import { Transform } from "stream";
import WebSocket from "ws";
import { Logger } from "./interfaces/logger";
import {
  PrivacyGroupWebSocketEvent,
  WebSocketClientOptions,
  WebSocketEvent,
  WebSocketEventCallback
} from "./interfaces/websocket";

abstract class PaladinWebSocketClientBase<
  TMessageTypes extends string,
  TEvent
> {
  private logger: Logger;
  private socket: WebSocket | undefined;
  private closed? = () => {};
  private pingTimer?: NodeJS.Timeout;
  private disconnectTimer?: NodeJS.Timeout;
  private reconnectTimer?: NodeJS.Timeout;
  private disconnectDetected = false;
  private counter = 1;

  constructor(
    private options: WebSocketClientOptions<TMessageTypes>,
    private callback: WebSocketEventCallback<TEvent>
  ) {
    this.logger = options.logger ?? console;
    this.connect();
  }

  private connect() {
    // Ensure we've cleaned up any old socket
    this.close();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      delete this.reconnectTimer;
    }

    const auth =
      this.options.username && this.options.password
        ? `${this.options.username}:${this.options.password}`
        : undefined;
    const socket = (this.socket = new WebSocket(this.options.url, {
      ...this.options.socketOptions,
      auth,
      handshakeTimeout: this.options.heartbeatInterval,
    }));
    this.closed = undefined;

    socket
      .on("open", () => {
        if (this.disconnectDetected) {
          this.disconnectDetected = false;
          this.logger.log("Connection restored");
        } else {
          this.logger.log("Connected");
        }
        this.schedulePing();
        for (const sub of this.options.subscriptions ?? []) {
          // Automatically connect subscriptions
          this.subscribe(sub.type, sub.name);
          this.logger.log(`Started listening on subscription ${sub.name}`);
        }
        if (this.options?.afterConnect !== undefined) {
          this.options.afterConnect(this);
        }
      })
      .on("error", (err) => {
        this.logger.error("Error", err.stack);
      })
      .on("close", () => {
        if (this.closed) {
          this.logger.log("Closed");
          this.closed(); // do this after all logging
        } else {
          this.disconnectDetected = true;
          this.reconnect("Closed by peer");
        }
      })
      .on("pong", () => {
        this.logger.debug && this.logger.debug(`WS received pong`);
        this.schedulePing();
      })
      .on("unexpected-response", (req, res: IncomingMessage) => {
        let responseData = "";
        res.pipe(
          new Transform({
            transform(chunk, encoding, callback) {
              responseData += chunk;
              callback();
            },
            flush: () => {
              this.reconnect(
                `Websocket connect error [${res.statusCode}]: ${responseData}`
              );
            },
          })
        );
      })
      .on("message", (data) => {
        const event: TEvent = JSON.parse(data.toString());
        this.callback(this, event);
      });
  }

  private clearPingTimers() {
    if (this.disconnectTimer) {
      clearTimeout(this.disconnectTimer);
      delete this.disconnectTimer;
    }
    if (this.pingTimer) {
      clearTimeout(this.pingTimer);
      delete this.pingTimer;
    }
  }

  private schedulePing() {
    this.clearPingTimers();
    const heartbeatInterval = this.options.heartbeatInterval ?? 30000;
    this.disconnectTimer = setTimeout(
      () => this.reconnect("Heartbeat timeout"),
      Math.ceil(heartbeatInterval * 1.5) // 50% grace period
    );
    this.pingTimer = setTimeout(() => {
      this.logger.debug && this.logger.debug(`WS sending ping`);
      this.socket?.ping("ping", true, (err) => {
        if (err) this.reconnect(err.message);
      });
    }, heartbeatInterval);
  }

  private reconnect(msg: string) {
    if (!this.reconnectTimer) {
      this.close();
      this.logger.error(`Websocket closed: ${msg}`);
      if (this.options.reconnectDelay === -1) {
        // do not attempt to reconnect
      } else {
        this.reconnectTimer = setTimeout(
          () => this.connect(),
          this.options.reconnectDelay ?? 5000
        );
      }
    }
  }

  send(json: object) {
    if (this.socket !== undefined) {
      this.socket.send(JSON.stringify(json));
    }
  }

  sendRpc(method: string, params: any[]) {
    this.send({
      jsonrpc: "2.0",
      id: this.counter++,
      method,
      params,
    });
  }

  async close(wait?: boolean): Promise<void> {
    const closedPromise = new Promise<void>((resolve) => {
      this.closed = resolve;
    });
    this.clearPingTimers();
    if (this.socket) {
      try {
        this.socket.close();
      } catch (e: any) {
        this.logger.warn(`Failed to clean up websocket: ${e.message}`);
      }
      if (wait) await closedPromise;
      this.socket = undefined;
    }
  }

  abstract subscribe(type: TMessageTypes, name: string): void;
  abstract ack(subscription: string): void;
  abstract nack(subscription: string): void;
}

export class PaladinWebSocketClient extends PaladinWebSocketClientBase<
  "receipts" | "blockchainevents",
  WebSocketEvent
> {
  subscribe(type: "receipts" | "blockchainevents", name: string) {
    this.sendRpc("ptx_subscribe", [type, name]);
  }

  ack(subscription: string) {
    this.sendRpc("ptx_ack", [subscription]);
  }

  nack(subscription: string) {
    this.sendRpc("ptx_nack", [subscription]);
  }
}

export class PrivacyGroupWebSocketClient extends PaladinWebSocketClientBase<
  "messages",
  PrivacyGroupWebSocketEvent
> {
  subscribe(type: "messages", name: string) {
    this.sendRpc("pgroup_subscribe", [type, name]);
  }

  ack(subscription: string) {
    this.sendRpc("pgroup_ack", [subscription]);
  }

  nack(subscription: string) {
    this.sendRpc("pgroup_nack", [subscription]);
  }
}
