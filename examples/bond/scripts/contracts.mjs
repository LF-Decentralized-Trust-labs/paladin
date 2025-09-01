  import { copyFile } from "copy-file";

  await copyFile(
    "../common/contracts/abis/BondSubscription.json",
    "src/abis/BondSubscription.json"
  );
  
  await copyFile(
    "../common/contracts/abis/BondTracker.json",
    "src/abis/BondTracker.json"
  );
  
  await copyFile(
    "../common/contracts/abis/InvestorList.json",
    "src/abis/InvestorList.json"
  );
  
  await copyFile(
    "../common/contracts/abis/BondTrackerPublic.json",
    "src/abis/BondTrackerPublic.json"
  );
  
  await copyFile(
    "../common/contracts/abis/AtomFactory.json",
    "src/abis/AtomFactory.json"
  );
  
  await copyFile(
    "../common/contracts/abis/Atom.json",
    "src/abis/Atom.json"
  );
  