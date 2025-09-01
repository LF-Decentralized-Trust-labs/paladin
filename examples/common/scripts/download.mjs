// download the zeto contracts
import { downloadFile, extractFile } from './util.mjs';

export async function downloadZetoAbis(zetoVersion = 'v0.2.0') {

    const zetoOrg = 'hyperledger-labs';
    const filename = `zeto-contracts-${zetoVersion}.tar.gz`;
    const url = `https://github.com/${zetoOrg}/zeto/releases/download/${zetoVersion}/${filename}`;
    const tmpFilePath = await downloadFile(url, filename);
    const tmpDir = await extractFile(tmpFilePath);
    return tmpDir;
}

// download the paladin contracts
export async function downloadPaladinAbis(paladinVersion = 'latest') {
    const paladinOrg = 'LF-Decentralized-Trust-labs';
    const filename = `abis.tar.gz`;
    let url;
    if (paladinVersion === "latest") {
        url = `https://github.com/${paladinOrg}/paladin/releases/latest/download/${filename}`;
    } else {
        url = `https://github.com/${paladinOrg}/paladin/releases/download/${paladinVersion}/${filename}`;
    }
    const tmpFilePath = await downloadFile(url, filename);
    const tmpDir = await extractFile(tmpFilePath, 'abis');
    return tmpDir;
}