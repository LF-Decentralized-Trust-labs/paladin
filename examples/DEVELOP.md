# Paladin Examples Development Guide

This guide outlines the process for developing Paladin examples and the requirements that all examples must meet.

## Overview

All Paladin examples must follow a consistent pattern that includes:
1. **Data Persistence**: Saving contract data to files for later verification
2. **Verification Scripts**: Ensuring deployed contracts remain accessible and functional
3. **Standard Commands**: Providing consistent npm scripts across all examples
4. **Documentation**: Clear README with setup and verification instructions

## Required Structure

Every example must have the following structure:

```
examples/your-example/
├── src/
│   ├── index.ts              # Main example implementation
│   ├── tests/
│   │   └── data-persistence.ts    # Data persistence verification script (REQUIRED)
│   └── abis/                 # Contract ABIs
├── package.json              # Required scripts
├── README.md                 # Clear instruction for running the example
└── tsconfig.json
```

## Required npm Scripts

Every example's `package.json` should include these scripts:

```json
{
  "scripts": {
    "build": "tsc",
    "start": "ts-node ./src/index.ts",
    "start:prod": "node ./build/index.js",
    "verify": "ts-node ./src/tests/data-persistence.ts",
    "abi": "node scripts/abi.mjs",
    "copy-abi": "node scripts/contracts.mjs"
  }
}
```

### Script Descriptions

- **`start`**: Run the example with ts-node
- **`start:prod`**: Run the example with built JavaScript
- **`verify`**: Run verification with ts-node
- **`abi`**: Copy ABIs from the solidity directory (this is for the scenario the solidity was built locally) 
- **`copy-abi`**: Copy ABIs from examples/common to example directory
 