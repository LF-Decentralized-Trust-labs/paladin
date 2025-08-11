import { copyFile } from "copy-file";

await copyFile(
  "../common/contracts/abis/Storage.json",
  "src/abis/Storage.json",
);