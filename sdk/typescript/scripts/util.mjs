import fs from 'fs';
import os from 'os';
import path from 'path';
import fetch from 'node-fetch';
import * as tar from 'tar';

export async function downloadFile(url, filename) {
  try {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'paladin-sdk'));
    const tmpFilePath = path.join(tmpDir, filename);
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const fileStream = fs.createWriteStream(tmpFilePath);
    await new Promise((resolve, reject) => {
      response.body.pipe(fileStream);
      response.body.on('error', reject);
      fileStream.on('finish', resolve);
    });
    console.log(`File downloaded successfully to: ${tmpFilePath}`);
    return tmpFilePath;
  } catch (error) {
    console.error('Error downloading file:', error);
  }
}

export async function extractFile(filePath, destinationDir) {
  try {
    let tmpDir = path.dirname(filePath); 
    let fullPath = tmpDir;
    if (destinationDir) {
      fullPath = path.join(tmpDir, destinationDir);
      fs.mkdirSync(fullPath, { recursive: true });
    }
    await tar.x({ file: filePath, C: fullPath });
    console.log(`File extracted successfully to: ${fullPath}`);
    return tmpDir;
  } catch (error) {
    console.error('Error extracting file:', error);
  }
}
