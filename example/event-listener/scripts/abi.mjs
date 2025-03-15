import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/tutorials/HelloWorld.sol/HelloWorld.json",
  "src/abis/HelloWorld.json"
);
