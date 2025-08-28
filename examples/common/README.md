# Common Package

The **common** package provides shared utilities for running the examples.
Before running any example, this package must be [built](#build).

## Build

```bash
npm install                                  # install dependencies
npm install @lfdecentralizedtrust-labs/paladin-sdk@latest
npm run download-abi                         # download ABIs
npm run build                                # build the 'common' package
```

## Configuration

Examples can be run against a Paladin node running locally or against a remote Paladin instance.

### Configuration Format

Configuration is defined in JSON files with the following structure:

```json
[
  {
    "name": "Node 1",
    "id": "node1",
    "client": {
      "url": "http://127.0.0.1:31548",
      "username": "optional-username",
      "password": "optional-password",
      "bearer": "optional-bearer-token",
      "tlsInsecure": false
    }
  }
]
```

### Field Reference

* `name` – Human-readable label for logs (e.g., `"Node 1"`, `"Central Bank"`)
* **id** – Node identifier used in verifier names (e.g., `"node1"`, `"node2"`)
* `client` – Connection details for the node:

  * `url` – Endpoint URL of the node (**required**)
  * `username` – Basic auth username (optional)
  * `password` – Basic auth password (optional)
  * `bearer` – Bearer token for authentication (optional)
  * `tlsInsecure` – Skip TLS certificate verification (default: `false`)

## Usage

### Default Configuration

By default, examples load configuration from `examples/common/config.json`:

```typescript
import { nodeConnections } from "../../common/src/config";

// Initialize Paladin clients
const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
const [paladin1, paladin2, paladin3] = clients;

// Create verifiers using the id from config
const [cashIssuer, bondIssuer] = paladin1.getVerifiers(
  `cashIssuer@${nodeConnections[0].id}`,
  `bondIssuer@${nodeConnections[0].id}`
);
const [bondCustodian] = paladin2.getVerifiers(`bondCustodian@${nodeConnections[1].id}`);
const [investor] = paladin3.getVerifiers(`investor@${nodeConnections[2].id}`);
```

### Custom Configuration

You can specify a custom config file when running examples:

```bash
npm run start -- -c /path/to/your/config.json
```

## Example Configurations

### Local Development (default)

```json
[
  {
    "name": "Node 1",
    "id": "node1",
    "client": { "url": "http://127.0.0.1:31548" }
  },
  {
    "name": "Node 2",
    "id": "node2",
    "client": { "url": "http://127.0.0.1:31648" }
  },
  {
    "name": "Node 3",
    "id": "node3",
    "client": { "url": "http://127.0.0.1:31748" }
  }
]
```

### Remote Environment with Authentication

```json
[
  {
    "name": "Remote Node 1",
    "id": "node1",
    "client": {
      "url": "https://evm-node-1.com/jsonrpc",
      "username": "your-username",
      "password": "your-password",
      "tlsInsecure": true
    }
  },
  {
    "name": "Remote Node 2",
    "id": "node2",
    "client": {
      "url": "https://evm-node-2.com/jsonrpc",
      "bearer": "your-bearer-token",
      "tlsInsecure": true
    }
  }
]
```
