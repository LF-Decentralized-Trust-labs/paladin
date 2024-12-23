import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/private/NotoTrackerERC20.sol/NotoTrackerERC20.json",
  "src/abis/NotoTrackerERC20.json"
);
