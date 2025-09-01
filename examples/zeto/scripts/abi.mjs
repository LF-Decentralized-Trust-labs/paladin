#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { copyFile } from 'copy-file';
import { downloadZetoAbis } from './download.mjs';

// download zeto and paladin abis and copy them to the contracts directory
const zetoDir = await downloadZetoAbis();

// create directory if it does not exist
fs.mkdirSync(path.join('src/zeto-abis'), { recursive: true });

// copy the zeto abis
await copyFile(path.join(zetoDir, 'artifacts/contracts/erc20.sol/SampleERC20.json'), 'src/zeto-abis/SampleERC20.json');
 