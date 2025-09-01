import { copyFile } from "copy-file";

await copyFile(
  "../common/contracts/abis/HelloWorld.json",
  "src/abis/HelloWorld.json"
);