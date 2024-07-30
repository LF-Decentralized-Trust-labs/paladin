import "@openzeppelin/hardhat-upgrades";
import "@nomicfoundation/hardhat-toolbox";
import "@typechain/hardhat";
import { HardhatUserConfig } from "hardhat/config";

const config: HardhatUserConfig = {
  solidity: {
    version: "0.8.20",
    settings: {
      viaIR: true,
      evmVersion: "berlin",
      optimizer: {
        enabled: true,
        runs: 1000,
      },
    },
  },
  defaultNetwork: "firefly",
  networks: {
    firefly: {
      url: "http://127.0.0.1:5100",
    },
    hardhat: {
      allowUnlimitedContractSize: true,
    },
  },
};

export default config;
