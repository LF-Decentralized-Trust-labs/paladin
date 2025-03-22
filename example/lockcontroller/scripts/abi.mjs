import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/private/NotoTrackerLockController.sol/NotoTrackerLockController.json",
  "src/abis/NotoTrackerLockController.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/shared/Atom.sol/AtomFactory.json",
  "src/abis/AtomFactory.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/shared/Atom.sol/Atom.json",
  "src/abis/Atom.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/shared/LockController.sol/LockController.json",
  "src/abis/LockController.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/shared/NotoERC20.sol/NotoERC20.json",
  "src/abis/NotoERC20.json"
);
