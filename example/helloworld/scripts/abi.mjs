import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/shared/HelloWorld.sol/HelloWorld.json",
  "src/abis/HelloWorld.json"
);