# Authentication and Authorization

Paladin supports optional RPC authorizers for securing JSON-RPC endpoints.

## Overview

RPC authorizers allow you to secure Paladin's RPC endpoints. They support both

*  __authentication__: using the HTTP headers to verify _who_ is making the request
*  __authorization__: using the JSON-RPC method and parameters to determine if the authenticated principal is permitted to perform a specific RPC operation

### Plugin Architecture

RPC authorizers are implemented as **Paladin plugins**, which means they:

* Run as **separate processes** communicating with the Paladin core via gRPC, providing isolation and security
* Follow the standard **Paladin plugin architecture** and lifecycle patterns
* Can be developed using the **plugin toolkit** (Go, Java, or other languages with gRPC support)
* Are loaded dynamically and can be configured per node without modifying core code

This plugin architecture provides several benefits:

* **Isolation**: Authorization plugin failures don't crash the core Paladin system
* **Security**: Authorization logic runs in separate processes, limiting potential attack surface
* **Extensibility**: New authorization schemes can be added by implementing custom plugins
* **Flexibility**: Different nodes can use different authorization plugins based on their requirements

The reference `basicauth` plugin demonstrates the standard plugin implementation pattern, which can be used as a template for creating custom authorization plugins.

### Multiple Authorizers

You can configure multiple authorizers that are executed sequentially in a chain. All authorizers in the chain must succeed for a request to proceed:

```json
{
  "rpcServer": {
    "authorizers": ["basicauth", "customauth"]
  }
}
```

When multiple authorizers are configured:

1. **Authentication phase**: Each authorizer's `Authenticate()` method is called in sequence. If any authorizer fails authentication, the request is rejected immediately.
2. **Authorization phase**: Each authorizer's `Authorize()` method is called in sequence with the corresponding authentication result. If any authorizer denies authorization, the request is rejected immediately.
3. All authorizers must succeed for the request to proceed to the RPC method handler.

### Request Lifecycle

#### HTTP Requests

For HTTP requests, both authentication and authorization happen for each request:

1. **Request arrives** → HTTP headers are extracted
2. **Authentication phase**: For each authorizer in the chain:
    - `Authenticate(headers)` is called
    - If authentication fails → Return HTTP 401 Unauthorized (native transport response, no JSON body) (stop processing)
    - Store the authentication result
3. **Authorization phase**: For each authorizer in the chain:
    - `Authorize(authenticationResult, method, payload)` is called
    - If authorization fails → Return JSON-RPC error response (stop processing)
4. **All authorizers succeed** → Proceed to RPC method handler

#### WebSocket Requests

For WebSocket connections, authentication happens once during the upgrade, and authorization happens for each RPC message:

**Connection Upgrade Phase:**

1. **Upgrade request arrives** → HTTP headers are extracted from the WebSocket upgrade request
2. **Authentication phase**: For each authorizer in the chain:
    - `Authenticate(headers)` is called before WebSocket upgrade
    - If authentication fails → Return HTTP 401 Unauthorized (native transport response, no JSON body), abort upgrade (stop processing)
    - Store the authentication result
3. **All authorizers succeed** → WebSocket upgrade proceeds (HTTP 101)
4. **Store authentication results** in the WebSocket connection context

**Subsequent RPC Messages:**

1. **RPC message arrives** → Retrieve stored authentication results from connection
2. **Authorization phase**: For each authorizer in the chain:
    - `Authorize(authenticationResult[i], method, payload)` is called
    - If authorization fails → Return JSON-RPC error response (stop processing)
3. **All authorizers succeed** → Proceed to RPC method handler

**Note**: WebSocket connections authenticate once during upgrade, but authorize each RPC message. This allows long-lived connections while still enforcing authorization checks on every operation. Authentication failures return native HTTP 401 responses, while authorization failures return JSON-RPC error responses.

## Basic Auth Plugin

The reference basic auth plugin implements HTTP Basic Authentication, where credentials are sent in the `Authorization` header of each HTTP request.

### Creating a Password File

Paladin uses standard bcrypt format compatible with `htpasswd`:

```bash
# Create a new password file
htpasswd -cB credentials.txt alice

# Add additional users
htpasswd -B credentials.txt bob
```

**Expected format**: Standard htpasswd bcrypt output
```
alice:$2a$10$N9qo8uLOickgx2ZMRZoMye...
bob:$2a$10$DaEm.rZiUdLSbnTdmHy9ve...
```

### Configuration

Add to your Paladin configuration:

```json
{
  "rpcAuthorizers": {
    "basicauth": {
      "plugin": {
        "library": "basicauth.so",
        "type": "rpc_auth"
      },
      "config": "{\"credentialsFile\": \"/path/to/users.txt\"}"
    }
  },
  "rpcServer": {
    "authorizers": ["basicauth"]
  }
}
```

**Key Fields**:
- `rpcAuthorizers`: Map of named authorization plugin configurations
- `rpcServer.authorizers`: Ordered array of authorizer plugin names to use

### Configuration via Helm Charts (Customnet)

When deploying Paladin nodes using Helm charts in `customnet` mode, RPC authorization can be configured directly through the values file.

**Important Deployment Workflow**

When enabling RPC authorization on a `customnet` deployment, the operator needs to submit transactions to deploy smart contracts and register nodes. These operations will fail if RPC authentication is enabled from the start, as the operator cannot authenticate to the RPC endpoints.

**Required Deployment Sequence:**

1. **Deploy without authentication**: Initially deploy your Paladin nodes **without** RPC authorization configured in the values file.

2. **Wait for deployment completion**: Verify that all smart contract deployments, transaction invocations, and node registrations have completed successfully:
   ```bash
   # Check smart contract deployments (all should show "Success" in STATUS column)
   kubectl get scd -n <your-namespace>
   
   # Check transaction invokes (all should show "Success" in STATUS column)
   kubectl get txn -n <your-namespace>
   
   # Check node registrations (all should show publishCount of 2 in Published column)
   kubectl get reg -n <your-namespace>
   ```

3. **Upgrade with authentication**: Once all deployments and registrations are complete, upgrade your Helm deployment with a values file that includes the `rpcAuth` configuration (see Step 2 below).

**Note**: If you attempt to deploy with RPC authorization enabled from the start, the operator will be unable to authenticate to the RPC endpoints, causing smart contract deployments, transaction invocations, and node registrations to fail.

#### Step 1: Create the Credentials Secret

First, create a Kubernetes secret containing the credentials file:

```bash
# Create the credentials file
htpasswd -cB credentials.htpasswd alice
# Enter password when prompted

# Add additional users (omit -c flag)
htpasswd -B credentials.htpasswd bob
# Enter password when prompted

# Create the Kubernetes secret
kubectl create secret generic paladin-basicauth-credentials \
  --from-file=credentials.htpasswd=./credentials.htpasswd \
  --namespace=<your-namespace>
```

**Important**: 
- The secret must be created in the same namespace where your Paladin nodes will be deployed
- The key **must** be named `credentials.htpasswd`
- The secret name can be anything you choose (e.g., `paladin-basicauth-credentials`)

#### Step 2: Configure in Values File (Upgrade Deployment)

After completing the initial deployment and verifying all smart contracts and registrations are complete (as described above), add the RPC authorization configuration to your `values-customnet.yaml` file and upgrade the deployment:

```yaml
paladinNodes:
  - name: "bank"
    # ... other configuration ...
    
    # Enable RPC authorization
    rpcAuth:
      secretName: paladin-basicauth-credentials  # Name of the existing secret
```

Then upgrade your Helm deployment:
```bash
helm upgrade <your-release-name> <chart-path> \
  -f values-customnet.yaml \
  -n <your-namespace>
```

When you configure `rpcAuth`, the operator automatically:
- Validates that the secret exists and contains the `credentials.htpasswd` key
- Mounts the credentials file at `/rpcauth/credentials.htpasswd` in the pod
- Configures the `basicauth` plugin with the correct path
- Sets `rpcServer.authorizers` to `["basicauth"]`

### Using Authentication

Once configured, all JSON-RPC requests require valid credentials:

```bash
# Make authenticated request
curl -u alice:password \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"account_list","id":1}' \
  http://localhost:8545
```

### WebSocket Connections

When using WebSocket connections, provide credentials in the initial upgrade request headers:

```bash
# Using wscat
wscat -H "Authorization: Basic $(echo -n 'alice:password' | base64)" \
  ws://localhost:8545
```

## Implementing Custom Auth Plugins

The reference `basicauth` plugin provides a complete example of implementing an RPC authorization plugin. This section explains how to create your own custom auth plugin following the same pattern.

### Plugin Structure

An RPC auth plugin must implement the `RPCAuthAPI` interface with three methods:

1. **`ConfigureRPCAuthorizer`**: Called once at startup to configure the plugin with settings
2. **`Authenticate`**: Called for each request to verify credentials from HTTP headers
3. **`Authorize`**: Called for each request to check if the authenticated principal is permitted to perform the operation

### Implementation Steps

#### Step 1: Create Plugin Directory Structure

Create a directory structure following the pattern used by `rpcauth/basicauth/`:

```
rpcauth/yourplugin/
├── yourplugin.go          # Main entry point with C exports
├── build.gradle           # Build configuration
├── go.mod                 # Go module definition
└── internal/
    └── yourplugin/
        ├── handler.go     # Implements RPCAuthAPI interface
        ├── config.go      # Configuration parsing
        └── ...            # Additional implementation files
```

#### Step 2: Implement the RPCAuthAPI Interface

Create a handler that implements the `RPCAuthAPI` interface:

```go
package yourplugin

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
    "github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// YourAuthHandler implements the RPCAuthAPI interface
type YourAuthHandler struct {
    // Your plugin state
}

var _ plugintk.RPCAuthAPI = (*YourAuthHandler)(nil)

// ConfigureRPCAuthorizer loads configuration
func (h *YourAuthHandler) ConfigureRPCAuthorizer(
    ctx context.Context,
    req *prototk.ConfigureRPCAuthorizerRequest,
) (*prototk.ConfigureRPCAuthorizerResponse, error) {
    // Parse configuration JSON
    var config YourConfig
    if err := json.Unmarshal([]byte(req.ConfigJson), &config); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }

    // Initialize your plugin state
    // ...

    return &prototk.ConfigureRPCAuthorizerResponse{}, nil
}

// Authenticate verifies credentials from HTTP headers
func (h *YourAuthHandler) Authenticate(
    ctx context.Context,
    req *prototk.AuthenticateRequest,
) (*prototk.AuthenticateResponse, error) {
    // Parse headers JSON
    var headers map[string]string
    if err := json.Unmarshal([]byte(req.HeadersJson), &headers); err != nil {
        return &prototk.AuthenticateResponse{
            Authenticated: false,
        }, nil
    }

    // Verify credentials (implement your authentication logic)
    // ...

    // Return authentication result (format is plugin-specific)
    // Example: JSON format
    result := map[string]string{"user": "alice", "role": "admin"}
    resultJSON, _ := json.Marshal(result)

    return &prototk.AuthenticateResponse{
        Authenticated: true,
        ResultJson:    stringPtr(string(resultJSON)),
    }, nil
}

// Authorize checks if authenticated principal is permitted
func (h *YourAuthHandler) Authorize(
    ctx context.Context,
    req *prototk.AuthorizeRequest,
) (*prototk.AuthorizeResponse, error) {
    // Parse authentication result (same format you returned from Authenticate)
    var authResult map[string]string
    if err := json.Unmarshal([]byte(req.ResultJson), &authResult); err != nil {
        return &prototk.AuthorizeResponse{
            Authorized: false,
        }, nil
    }

    // Check permissions based on:
    // - Authentication result (req.ResultJson)
    // - RPC method (req.Method)
    // - Request payload (req.PayloadJson)
    // ...

    // Return authorization decision
    return &prototk.AuthorizeResponse{
        Authorized: true,
    }, nil
}

func stringPtr(s string) *string {
    return &s
}
```

**Key Points**:
- The authentication result format is entirely plugin-specific (JSON, plain string, etc.)
- The same result string returned from `Authenticate()` is passed to `Authorize()` unchanged
- `Authenticate()` returns only `authenticated` (bool) and optionally `result_json` (when authenticated=true)
- `Authorize()` returns only `authorized` (bool)
- Authentication failures return native HTTP 401 responses (not JSON-RPC errors)
- Authorization failures in WebSocket messages return JSON-RPC error responses

#### Step 3: Create Main Entry Point

Create the main entry point with C exports for the plugin loader:

```go
package main

import (
    "C"

    "github.com/LFDT-Paladin/paladin/rpcauth/yourplugin/internal/yourplugin"
    "github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
)

var ple = plugintk.NewPluginLibraryEntrypoint(func() plugintk.PluginBase {
    return plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
        return &yourplugin.YourAuthHandler{}
    })
})

//export Run
func Run(grpcTargetPtr, pluginUUIDPtr *C.char) int {
    return ple.Run(
        C.GoString(grpcTargetPtr),
        C.GoString(pluginUUIDPtr),
    )
}

//export Stop
func Stop(pluginUUIDPtr *C.char) {
    ple.Stop(C.GoString(pluginUUIDPtr))
}

func main() {}
```

#### Step 4: Configure Build Integration

Add your plugin to the Paladin build system:

**In `build.gradle`** (root):
```groovy
def rpcAuths = [
    'rpcauth/basicauth/build/libs',
    'rpcauth/yourplugin/build/libs',  // Add your plugin
]

def assembleSubprojects = [
    // ...
    ':rpcauth:yourplugin',  // Add your plugin
]
```

**In `Dockerfile`**:
```dockerfile
FROM base-builder AS full-builder
...
COPY rpcauth/yourplugin rpcauth/yourplugin
...
```

**Create `build.gradle`** in your plugin directory:
```groovy
ext {
    goFiles = fileTree('.') {
        include 'internal/**/*.go'
        include 'yourplugin.go'
    }
}

configurations {
    toolkitGo {
        canBeConsumed = false
        canBeResolved = true
    }
    goSource {
        canBeConsumed = true
        canBeResolved = false
    }
    yourplugin {
        canBeConsumed = true
        canBeResolved = false
    }
}

dependencies {
    toolkitGo project(path: ':toolkit:go', configuration: 'goSource')
}

task buildGo(type: GoLib, dependsOn: [':toolkit:go:protoc']) {
    inputs.files(configurations.toolkitGo)
    baseName 'yourplugin'
    sources goFiles
    mainFile 'yourplugin.go'
}

task assemble {
    dependsOn buildGo
}

dependencies {
    yourplugin files(buildGo)
    goSource files(goFiles)
}
```

#### Step 5: Configure and Use Your Plugin

Add your plugin to the Paladin configuration:

```json
{
  "rpcAuthorizers": {
    "yourplugin": {
      "plugin": {
        "library": "yourplugin.so",
        "type": "c-shared"
      },
      "config": "{\"yourConfigKey\": \"yourConfigValue\"}"
    }
  },
  "rpcServer": {
    "authorizers": ["yourplugin"]
  }
}
```

### Reference Implementation

The `rpcauth/basicauth/` directory provides a complete reference implementation:

- `basicauth.go`: Main entry point
- `internal/basicauth/handler.go`: Implements `RPCAuthAPI` interface
- `internal/basicauth/config.go`: Configuration parsing
- `internal/basicauth/authenticator.go`: Credential management
