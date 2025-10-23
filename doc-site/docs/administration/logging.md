# Logging Guide

This guide covers Paladin's logging system, including component-based logging and how to filter logs in Kubernetes environments.

## Overview

Paladin uses structured logging with component-based context to provide clear visibility into system operations. Each log entry includes a `component` field that identifies which part of the system generated the log message.

## Log Level Configuration

### Setting Log Levels

Log levels are configured in the YAML configuration file under the `log` section.

```yaml
log:
  level: debug
```

### Available Log Levels

| Level | Description | Use Case |
|-------|-------------|----------|
| `trace` | Most detailed information for debugging | Detailed tracing of function calls and internal operations |
| `debug` | Detailed information for debugging | Development and troubleshooting |
| `info` | General information about system operation | Normal operation monitoring |
| `warn` | Warning messages for potentially harmful situations | Issues that don't stop operation |
| `error` | Error messages for failed operations | Critical issues requiring attention |

## Log Structure

Paladin logs are structured text with the following key fields:

- **`timestamp`**: ISO 8601 timestamp in brackets `[2025-09-16T14:02:51.439Z]`
- **`level`**: Log level (TRACE, DEBUG, INFO, WARN, ERROR)
- **`message`**: Human-readable log message
- **`component`**: Identifies the system component (see table below)
- **`pid`**: Process ID
- **`role`**: Additional context about the operation role
- **Additional fields**: Component-specific context (transaction IDs, addresses, etc.)

### Component Reference

The following table lists all components that are included in Paladin logs based on actual `log.WithComponent` calls in the codebase:

| Component | Description |
|-----------|-------------|
| `blockindexer` | Monitors blockchain events and indexes transactions |
| `domainmanager` | Manages domain plugins and smart contracts |
| `evmregistry` | EVM-based registry implementation |
| `examplesigningmodule` | Example signing module for testing |
| `grpctransport` | gRPC-based transport implementation |
| `groupmanager` | Group management system |
| `identityresolver` | Identity resolution service |
| `keymanager` | Key management system |
| `keyresolver` | Key resolution utilities |
| `noto` | Noto domain implementation |
| `pluginmanager` | Plugin management system |
| `privatetxnmanager` | Private transaction manager |
| `publictxnmanager` | Public transaction manager |
| `registrymanager` | Registry management system |
| `schema` | Schema management utilities |
| `statemanager` | State management system |
| `staticregistry` | Static registry implementation |
| `transportmanager` | Transport management system |
| `txmanager` | Transaction manager |
| `zeto` | Zeto domain implementation |

## Kubernetes Log Filtering

### Basic Log Filtering

To filter logs by component in Kubernetes:

```bash
# Filter logs for a specific component
kubectl logs -l app=paladin | grep "component=txmanager"

# Filter logs for multiple components
kubectl logs -l app=paladin | grep -E "component=(txmanager|statemanager)"

# Filter logs excluding certain components
kubectl logs -l app=paladin | grep -v "component=retry"
```

### Filtering by Log Level

```bash
# Show only error logs
kubectl logs -l app=paladin  -l app.kubernetes.io/instance=node1 | grep "ERROR"

# Show warnings and errors
kubectl logs -l app=paladin  -l app.kubernetes.io/instance=node1 | grep -E "(WARN|ERROR)"

# Exclude debug logs
kubectl logs -l app=paladin  -l app.kubernetes.io/instance=node1 | grep -v "DEBUG"
```

### Real-time Log Monitoring

For real-time monitoring of specific components:

```bash
# Follow logs for transaction manager in real-time
kubectl logs -f -l app=paladin -l app.kubernetes.io/instance=node1 | grep "component=txmanager"

# Follow error logs from all components
kubectl logs -f -l app=paladin -l app.kubernetes.io/instance=node1 | grep "ERROR"

# Follow logs for multiple components
kubectl logs -f -l app=paladin -l app.kubernetes.io/instance=node1 | grep -E "component=(txmanager|statemanager|domainmanager)"
```


