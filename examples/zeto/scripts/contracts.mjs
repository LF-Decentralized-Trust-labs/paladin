import { copyFile } from "copy-file";

// Copy the SampleERC20 ABI from SDK
await copyFile(
  "../common/contracts/zeto-abis/SampleERC20.json",
  "src/zeto-abis/SampleERC20.json"
);
 