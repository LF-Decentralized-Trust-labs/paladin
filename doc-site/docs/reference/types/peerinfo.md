---
title: PeerInfo
---
{% include-markdown "./_includes/peerinfo_description.md" %}

### Example

```json
{
    "name": "",
    "stats": {
        "sentMsgs": 0,
        "receivedMsgs": 0,
        "sentBytes": 0,
        "receivedBytes": 0,
        "lastSend": null,
        "lastReceive": null,
        "reliableHighestSent": 0,
        "reliableAckBase": 0
    },
    "activated": 0
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `name` | The name of the peer node | `string` |
| `stats` | Statistics for the outbound and inbound data transfer | [`PeerStats`](#peerstats) |
| `activated` | The time when the peer was activated by an attempt to send data, or data arriving on a transport from this peer | [`Timestamp`](simpletypes.md#timestamp) |
| `outboundTransport` | The name of the transport selected for outbound connection to the peer. Omitted if no attempt to send data has occurred for this peer | `string` |
| `outbound` | Transport specific information about an established outbound connection to the peer. Omitted if the peer does not have an established outbound connection | `` |
| `outboundError` | Contains an error if attempting to send data, and the transport connection failed | `error` |

## PeerStats

| Field Name | Description | Type |
|------------|-------------|------|
| `sentMsgs` | Count of messages sent since activation of this peer | `uint64` |
| `receivedMsgs` | Count of messages received since activation of this peer | `uint64` |
| `sentBytes` | Count of payload bytes sent since activation of this peer (does not include header data) | `uint64` |
| `receivedBytes` | Count of payload bytes received since activation of this peer (does not include header data) | `uint64` |
| `lastSend` | Timestamp of the last send to this peer | [`Timestamp`](simpletypes.md#timestamp) |
| `lastReceive` | Timestamp of the last receive from this peer | [`Timestamp`](simpletypes.md#timestamp) |
| `reliableHighestSent` | Outbound reliable messages are assigned a sequence. This is the highest sequence sent to the peer since activation | `uint64` |
| `reliableAckBase` | Outbound reliable messages are assigned a sequence. This is the lowest sequence that has not received an acknowledgement from the peer | `uint64` |


