import { copyFile } from 'copy-file';
import path from 'path';
import { downloadFile, extractFile } from './util.mjs';

await copyFile('../../solidity/artifacts/contracts/domains/pente/PentePrivacyGroup.sol/PentePrivacyGroup.json', 'src/domains/abis/PentePrivacyGroup.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/INoto.sol/INoto.json', 'src/domains/abis/INoto.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/INotoPrivate.sol/INotoPrivate.json', 'src/domains/abis/INotoPrivate.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/IZetoFungible.sol/IZetoFungible.json', 'src/domains/abis/IZetoFungible.json');

// download the zeto anon contract ABI
const zetoVersion = 'v0.0.12';
const zetoOrg = 'hyperledger-labs';
const filename = `zeto-contracts-${zetoVersion}.tar.gz`;
const url = `https://github.com/${zetoOrg}/zeto/releases/download/${zetoVersion}/${filename}`;
const tmpFilePath = await downloadFile(url, filename);
const tmpDir = await extractFile(tmpFilePath);

// copy the zeto anon contract ABI
await copyFile(path.join(tmpDir, 'artifacts/contracts/zeto_anon.sol/Zeto_Anon.json'), 'src/domains/abis/Zeto_Anon.json');
await copyFile(path.join(tmpDir, 'artifacts/contracts/erc20.sol/SampleERC20.json'), 'src/domains/abis/SampleERC20.json');
