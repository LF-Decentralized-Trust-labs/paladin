import { PaladinConfig } from "@lfdecentralizedtrust-labs/paladin-sdk";
import https from "https";
import fs from "fs";
import path from "path";


export const DEFAULT_POLL_TIMEOUT = 30000;
export const LONG_POLL_TIMEOUT = 120000;
export const POLL_INTERVAL = 1000;

/**
 * Defines the connectivity information for a single network node in JSON format.
 */
export interface NodeConnectionJson {
  name: string; // A human-readable name for logging, e.g., "Node 1" or "Central Bank"
  id: string; // The node identifier used in verifier names, e.g., "node1", "node2", "node3"
  client: {
    url: string;
    username?: string;
    password?: string;
    bearer?: string;
    tlsInsecure?: boolean;
  };
}

/**
 * Defines the connectivity information for a single network node.
 */
export interface NodeConnection {
  name: string; // A human-readable name for logging, e.g., "Node 1" or "Central Bank"
  id: string; // The node identifier used in verifier names, e.g., "node1", "node2", "node3"
  clientOptions: PaladinConfig; // The options for the PaladinClient constructor
}

/**
 * Converts a NodeConnectionJson to NodeConnection
 */
function convertJsonToNodeConnection(jsonNode: NodeConnectionJson): NodeConnection {
  const clientOptions: PaladinConfig = {
    url: jsonNode.client.url,
  };

  // Add authentication if provided
  if (jsonNode.client.username && jsonNode.client.password) {
    const auth = Buffer.from(`${jsonNode.client.username}:${jsonNode.client.password}`).toString('base64');
    clientOptions.requestConfig = {
      headers: {
        Authorization: `Basic ${auth}`
      }
    };
  } else if (jsonNode.client.bearer) {
    clientOptions.requestConfig = {
      headers: {
        Authorization: `Bearer ${jsonNode.client.bearer}`
      }
    };
  }

  // Add TLS configuration if tlsInsecure is true
  if (jsonNode.client.tlsInsecure) {
    clientOptions.requestConfig = {
      ...clientOptions.requestConfig,
      httpsAgent: new https.Agent({ rejectUnauthorized: false })
    };
  }

  return {
    name: jsonNode.name,
    id: jsonNode.id,
    clientOptions
  };
}

/**
 * Loads the configuration from a JSON file
 */
function loadConfigFromFile(configPath?: string): NodeConnection[] {
  let filePath: string;
  
  if (configPath) {
    // Use the provided config path
    filePath = path.resolve(configPath);
  } else {
    // Use the default config file - try multiple possible locations
    const possiblePaths = [
      path.join(__dirname, '..', 'config.json'), // build/src/../config.json
      path.join(__dirname, '..', '..', 'config.json'), // build/src/../../config.json
      path.join(process.cwd(), 'config.json'), // current working directory
      path.join(process.cwd(), 'examples', 'common', 'config.json'), // examples/common/config.json
    ];
    
    const foundPath = possiblePaths.find(p => fs.existsSync(p));
    
    if (!foundPath) {
      throw new Error(`Configuration file not found. Tried: ${possiblePaths.join(', ')}`);
    }
    
    filePath = foundPath;
  }

  if (!fs.existsSync(filePath)) {
    throw new Error(`Configuration file not found: ${filePath}`);
  }

  try {
    const fileContent = fs.readFileSync(filePath, 'utf8');
    const jsonConfig: NodeConnectionJson[] = JSON.parse(fileContent);
    
    if (!Array.isArray(jsonConfig)) {
      throw new Error('Configuration file must contain an array of node connections');
    }

    return jsonConfig.map(convertJsonToNodeConnection);
  } catch (error) {
    if (error instanceof SyntaxError) {
      throw new Error(`Invalid JSON in configuration file: ${filePath}`);
    }
    throw error;
  }
}

/**
 * Gets the config path from command line arguments
 */
function getConfigPathFromArgs(): string | undefined {
  const args = process.argv.slice(2);
  const configIndex = args.findIndex(arg => arg === '--config' || arg === '-c');
  
  if (configIndex !== -1 && configIndex + 1 < args.length) {
    return args[configIndex + 1];
  }
  
  // Also check for positional argument (last argument)
  if (args.length > 0 && !args[args.length - 1].startsWith('-')) {
    return args[args.length - 1];
  }
  
  return undefined;
}

/**
 * Get the configuration, optionally from a custom path
 */
export function getNodeConnections(configPath?: string): NodeConnection[] {
  const pathToUse = configPath || getConfigPathFromArgs();
  return loadConfigFromFile(pathToUse);
}

// Default configuration (loaded from config.json or command line)
const nodeConnections: NodeConnection[] = getNodeConnections();

// Export the default configuration
export default nodeConnections;

// Export both configurations for flexibility
export { nodeConnections };