import { copyFile } from 'copy-file';

await copyFile('../../sdk/typescript/src/domains/abis/SampleERC20.json', 'src/abis/SampleERC20.json');
