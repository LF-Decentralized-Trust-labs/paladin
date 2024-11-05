import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/private/BondSubscription.sol/BondSubscription.json",
  "src/abis/BondSubscription.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/private/BondTracker.sol/BondTracker.json",
  "src/abis/BondTracker.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/private/InvestorRegistry.sol/InvestorRegistry.json",
  "src/abis/InvestorRegistry.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/shared/BondTrackerPublic.sol/BondTrackerPublic.json",
  "src/abis/BondTrackerPublic.json"
);
