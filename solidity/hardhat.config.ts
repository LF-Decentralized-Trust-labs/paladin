import "@nomicfoundation/hardhat-toolbox";
import "@openzeppelin/hardhat-upgrades";
import "@typechain/hardhat";
import "hardhat-dependency-compiler";
import { HardhatUserConfig } from "hardhat/config";

const config: HardhatUserConfig = {
  solidity: {
    version: "0.8.20",
    settings: {
      evmVersion: "paris",
      optimizer: {
        enabled: true,
        runs: 1000,
      },
    },
  },
  dependencyCompiler: {
    paths: [
      "zeto/contracts/zeto_anon.sol",
      "zeto/contracts/zeto_anon_enc.sol",
      "zeto/contracts/zeto_anon_enc_nullifier.sol",
      "zeto/contracts/zeto_anon_nullifier.sol",
      "zeto/contracts/zeto_nf_anon.sol",
      "zeto/contracts/zeto_nf_anon_nullifier.sol",
    ],
  },
};

export default config;
