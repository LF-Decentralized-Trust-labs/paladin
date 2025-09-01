#!/usr/bin/env node

// This script downloads the latest stable solidity contracts and copies them to the destination directory
//
// Usage:
//   node scripts/contracts.mjs
//
// ENV variables:
//   PALADIN_ABI_VERSION: the version of the Paladin contracts to download (default: latest)
//   ZETO_ABI_VERSION: the version of the Zeto contracts to download (default: v0.2.0)

import fs from 'fs';
import path from 'path';
import { copyFile} from 'copy-file';
import { downloadPaladinAbis, downloadZetoAbis } from './download.mjs';

const destinationDir = "contracts";
const paladinVersion = process.env.PALADIN_ABI_VERSION || "latest";
const zetoVersion = process.env.ZETO_ABI_VERSION || "v0.2.0";

console.log(`Paladin version: ${paladinVersion}`);
console.log(`Zeto version: ${zetoVersion}`);

fs.mkdirSync(path.join(destinationDir, 'abis'), { recursive: true });
fs.mkdirSync(path.join(destinationDir, 'zeto-abis'), { recursive: true });

// download the paladin contracts
const paladinDir = await downloadPaladinAbis(paladinVersion);
// copy the abis directory to the destination directory
const paladinFiles = fs.readdirSync(path.join(paladinDir, 'abis'));
for (const file of paladinFiles) {
    if (file.endsWith('.json')) {
        await copyFile(path.join(paladinDir, 'abis', file), path.join(destinationDir, 'abis', file));
    }
}

// Download Zeto contracts
const zetoDir = await downloadZetoAbis(zetoVersion);
const zetoFiles = fs.readdirSync(zetoDir, { recursive: true });
for (const file of zetoFiles) {
    if (file.endsWith('.json') && !file.endsWith('.dbg.json')) {
        // flatten the directory structure
        const fileName = path.basename(file);
        const destinationPath = path.join(destinationDir, 'zeto-abis', fileName);
        await copyFile(path.join(zetoDir, file), destinationPath);
    }
}

console.log("\nContract download completed successfully!");
console.log(`Paladin contracts: ${destinationDir}/abis`);
console.log(`Zeto contracts: ${destinationDir}/zeto-abis`);

