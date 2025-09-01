import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/tutorials/Storage.sol/Storage.json",
  "src/abis/Storage.json",
);