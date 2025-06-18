### Create receipt listener

```js
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "ptx_createReceiptListener",
    "params": [{
        "name":"listener1",
        "filters": {
            "sequenceAbove": null,
            "type": "private",
            "domain": "pente"
        },
        "options": {
            "incompleteStateReceiptBehavior": "block_contract",
            "domainReceipts": true
        }
    }]
}
```

Note the ability to filter on particular receipt types, and most importantly the ability to control delivery behavior when states are incomplete:

- **`block_contract`** (default): Pauses delivery for each individual smart contract address when incomplete states are detected
- **`process`**: Delivers all receipts immediately, regardless of what private state data is available
- **`complete_only`**: Delivers receipts whenever the domain confirms all expected states are complete, without regard for strict ordering

### Subscribe (WebSockets only)

```js
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "ptx_subscribe",
    "params": ["receipts", "listener1"]
}
```

### Ack 

Confirms receipt of the last batch for this subscription ID (which changes on each ptx_subscribe), so the next batch is delivered.

```js
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "ptx_ack",
    "params": ["5b3e0816-32e2-4aa8-80e6-6d2e41e046cb"]
}
```

> No reply is sent to `ptx_ack` - only the next batch

### Nack

Drives redelivery for the last batch.

```js
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "ptx_nack",
    "params": ["5b3e0816-32e2-4aa8-80e6-6d2e41e046cb"]
}
```

> No reply is sent to `ptx_ack` - only the redelivery batch

### Unsubscribe

```js
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "ptx_unsubscribe",
    "params": ["5b3e0816-32e2-4aa8-80e6-6d2e41e046cb"]
}
```

### Delete receipt listener

```js
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "ptx_deleteReceiptListener",
    "params": ["listener1"]
}
```