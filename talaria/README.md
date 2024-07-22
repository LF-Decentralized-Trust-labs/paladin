## Talaria

[Talaria](https://en.wikipedia.org/wiki/Talaria) the winged sandals of the messenger Hermes. 

This folder serves as an implementation of the diagram below, with fake versions of some major components to demonstrate messaging flow.

![](img/talaria.png)


### Getting started (Comms)

To get this model working, 2 instances of the main script will be required. This can be done as so:

```sh
go run main.go -commsbusport 5050 -registryport 5051 -talariaport 5052
```

Where:
  - `commsbusport` is the port for the API to feed the comms bus with messages (only exists for dev)
  - `registryport` is the port for the API to feed the registry with known peers (only exists for dev)
  - `talariaport` is the port for the gRPC endpoint that other paladins will connect to (will actually exist)

This script will need to be started in 2 different terminal sessions.

Once the server is initialised, an API call will need to be made to register a node peer in the registry caravan and then
messages can be sent. To do this, make a call like the following:

```sh
curl --location 'localhost:5051/peer' \
--header 'Content-Type: application/json' \
--data-raw '{
    "routingInformation": "{\"address\":\"localhost:8082\"}",
    "transactingEntity": "sam@node1"
}'
```

> **NOTE:** If you're using localhost here, the port is the port of the Talaria on the other node

Then messages can be sent through the comms bus by making a call like the following:

```sh
curl --location 'localhost:5050/message' \
--header 'Content-Type: application/json' \
--data-raw '{
    "to": "sam@node1",
    "content": "Hello, World!"
}'
```

### Getting started (Development)

`make proto` to generate the protobuf client files for the interpaladin transport, and the Talaria -> Plugin transport