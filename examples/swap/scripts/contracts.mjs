import { copyFile } from "copy-file";

await copyFile(
  "../common/contracts/abis/NotoTrackerERC20.json",
  "src/abis/NotoTrackerERC20.json"
);

await copyFile(
  "../common/contracts/abis/AtomFactory.json",
  "src/abis/AtomFactory.json"
);

await copyFile(
  "../common/contracts/abis/Atom.json",
  "src/abis/Atom.json"
);
