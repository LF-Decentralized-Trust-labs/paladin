import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/domains/pente/PentePrivacyGroup.sol/PentePrivacyGroup.json",
  "src/domains/abis/PentePrivacyGroup.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/domains/interfaces/INoto.sol/INoto.json",
  "src/domains/abis/INoto.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/domains/interfaces/INotoPrivate.sol/INotoPrivate.json",
  "src/domains/abis/INotoPrivate.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/domains/interfaces/IZetoFungible.sol/IZetoFungible.json",
  "src/domains/abis/IZetoFungible.json"
);

await copyFile(
  "../../domains/zeto/integration-test/abis/Zeto_Anon.json",
  "src/domains/abis/Zeto_Anon.json"
);
