#!/usr/bin/env node

import path from 'path';
import { copyFile } from 'copy-file';
import { downloadZetoAbis, downloadPaladinAbis } from './download.mjs';

// download zeto and paladin abis and copy them to the contracts directory
const zetoDir = await downloadZetoAbis();
const paladinDir = await downloadPaladinAbis();

// copy the zeto abis
await copyFile(path.join(zetoDir, 'artifacts/contracts/zeto_anon.sol/Zeto_Anon.json'), 'src/domains/abis/Zeto_Anon.json');
await copyFile(path.join(zetoDir, 'artifacts/contracts/lib/interfaces/izeto_kyc.sol/IZetoKyc.json'), 'src/domains/abis/IZetoKyc.json');
await copyFile(path.join(zetoDir, 'artifacts/contracts/erc20.sol/SampleERC20.json'), 'src/domains/abis/SampleERC20.json');

// copy the paladin abis
await copyFile(path.join(paladinDir, 'abis/PentePrivacyGroup.json'), 'src/domains/abis/PentePrivacyGroup.json');
await copyFile(path.join(paladinDir, 'abis/INoto.json'), 'src/domains/abis/INoto.json');
await copyFile(path.join(paladinDir, 'abis/INotoPrivate.json'), 'src/domains/abis/INotoPrivate.json');
await copyFile(path.join(paladinDir, 'abis/IZetoFungible.json'), 'src/domains/abis/IZetoFungible.json');