# Common Configuration

This directory contains the centralized configuration for all Paladin examples using a user-friendly JSON format.

## Configuration Structure

The configuration is stored in JSON files with the following structure:

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

### Configuration Fields

- `name`: A human-readable name for logging (e.g., "Node 1", "Central Bank")
- `id`: The node identifier used in verifier names (e.g., "node1", "node2", "node3")
- `client`: Client configuration object
  - `url`: The endpoint URL for the node (required)
  - `username`: Username for Basic authentication (optional)
  - `password`: Password for Basic authentication (optional)
  - `bearer`: Bearer token for authentication (optional)
  - `tlsInsecure`: Whether to skip TLS certificate verification (optional, default: false)

## Usage

### Default Configuration

By default, examples will use `examples/common/config.json`:

```typescript
import { nodeConnections } from "../../common/src/config";

// Initialize Paladin clients from the environment configuration
const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
const [paladin1, paladin2, paladin3] = clients;

// Create verifiers using the id from config and your specific verifier names
// Keep original verifier names, just use config id for creation
const [cashIssuer, bondIssuer] = paladin1.getVerifiers(
  `cashIssuer@${nodeConnections[0].id}`,
  `bondIssuer@${nodeConnections[0].id}`
);
const [bondCustodian] = paladin2.getVerifiers(`bondCustodian@${nodeConnections[1].id}`);
const [investor] = paladin3.getVerifiers(`investor@${nodeConnections[2].id}`);
```

### Custom Configuration

You can specify a custom configuration file when running examples:

```bash
# Using -c/--config flag
npm run start -- -c /path/to/your/config.json
```

### Example Configurations

#### Local Development (default)
```json
[
  {
    "name": "Node 1",
    "id": "node1",
    "client": {
      "url": "http://127.0.0.1:31548"
    }
  },
  {
    "name": "Node 2",
    "id": "node2",
    "client": {
      "url": "http://127.0.0.1:31648"
    }
  },
  {
    "name": "Node 3",
    "id": "node3",
    "client": {
      "url": "http://127.0.0.1:31748"
    }
  }
]
```

#### Remote Environment with Authentication
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
