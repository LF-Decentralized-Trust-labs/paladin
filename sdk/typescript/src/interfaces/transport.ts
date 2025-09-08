export interface IPeerInfo {
  name: string;
  stats: IPeerStats;
  activated: string;
  outboundTransport?: string;
  outbound?: any;
  outboundError?: any;
}

export interface IPeerStats {
  sentMsgs: number;
  receivedMsgs: number;
  sentBytes: number;
  receivedBytes: number;
  lastSend?: string;
  lastReceive?: string;
  reliableHighestSent: number;
  reliableAckBase: number;
}

export interface IReliableMessage {
  sequence: number;
  id: string;
  created: string;
  node: string;
  messageType: string;
  metadata: any;
  ack?: IReliableMessageAck;
}

export interface IReliableMessageAck {
  messageId: string;
  time?: string;
  error?: string;
} 