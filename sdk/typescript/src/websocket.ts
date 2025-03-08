import { IncomingMessage } from "http";
import { Transform } from "stream";
import WebSocket from "ws";
import { Logger } from "./interfaces/logger";
import {
  WebSocketClientOptions,
  WebSocketEvent,
  WebSocketEventCallback,
} from "./interfaces/websocket";

export class PaladinWebSocketClient {
  private logger: Logger;
  private socket: WebSocket | undefined;
  private closed? = () => {};
  private pingTimer?: NodeJS.Timeout;
  private disconnectTimer?: NodeJS.Timeout;
  private reconnectTimer?: NodeJS.Timeout;
  private disconnectDetected = false;
  private counter = 1;

  constructor(
    private options: WebSocketClientOptions,
    private callback: WebSocketEventCallback
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
        for (const name of this.options.subscriptions) {
          this.send({
            jsonrpc: "2.0",
            id: this.counter++,
            method: "ptx_subscribe",
            params: ["receipts", name],
          });
          this.logger.log(`Started listening on subscription ${name}`);
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
        const event: WebSocketEvent = JSON.parse(data.toString());
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

  ack(subscription: string) {
    this.send({
      jsonrpc: "2.0",
      id: this.counter++,
      method: "ptx_ack",
      params: [subscription],
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
}
