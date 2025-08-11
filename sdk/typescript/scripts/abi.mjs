import { copyFile } from 'copy-file';
import path from 'path';
import { downloadZetoAbis } from './download.mjs';

await copyFile('../../solidity/artifacts/contracts/domains/pente/PentePrivacyGroup.sol/PentePrivacyGroup.json', 'src/domains/abis/PentePrivacyGroup.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/INoto.sol/INoto.json', 'src/domains/abis/INoto.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/INotoPrivate.sol/INotoPrivate.json', 'src/domains/abis/INotoPrivate.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/IZetoFungible.sol/IZetoFungible.json', 'src/domains/abis/IZetoFungible.json');

// download the zeto anon contract ABI
const tmpDir = await downloadZetoAbis();

// copy the zeto anon contract ABI
await copyFile(path.join(tmpDir, 'artifacts/contracts/zeto_anon.sol/Zeto_Anon.json'), 'src/domains/abis/Zeto_Anon.json');
await copyFile(path.join(tmpDir, 'artifacts/contracts/lib/interfaces/izeto_kyc.sol/IZetoKyc.json'), 'src/domains/abis/IZetoKyc.json');
await copyFile(path.join(tmpDir, 'artifacts/contracts/erc20.sol/SampleERC20.json'), 'src/domains/abis/SampleERC20.json');
