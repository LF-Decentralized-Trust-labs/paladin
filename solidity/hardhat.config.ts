import "@nomicfoundation/hardhat-toolbox";
import "@openzeppelin/hardhat-upgrades";
import "@typechain/hardhat";
import { HardhatUserConfig } from "hardhat/config";

const config: HardhatUserConfig = {
  solidity: {
    version: "0.8.30",
    settings: {
      evmVersion: "shanghai",
      optimizer: {
        enabled: false,
        runs: 1000,
      },
    },
  },
};

export default config;
