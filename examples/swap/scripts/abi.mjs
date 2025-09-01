import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/private/NotoTrackerERC20.sol/NotoTrackerERC20.json",
  "src/abis/NotoTrackerERC20.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/shared/AtomFactory.sol/AtomFactory.json",
  "src/abis/AtomFactory.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/shared/Atom.sol/Atom.json",
  "src/abis/Atom.json"
);
