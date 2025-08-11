import { copyFile } from "copy-file";

// Copy the KYC interface ABI from zeto domain
await copyFile(
  "../common/contracts/zeto-abis/IZetoKyc.json",
  "src/zeto-abis/IZetoKyc.json"
);

// Copy the SampleERC20 ABI from SDK
await copyFile(
  "../common/contracts/zeto-abis/SampleERC20.json",
  "src/zeto-abis/SampleERC20.json"
);
