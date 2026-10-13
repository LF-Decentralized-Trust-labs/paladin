import { readFileSync } from "fs";
import { ethers } from "hardhat";

/**
 * Helper function to deploy an upgradeable contract using UUPS proxy pattern
 * @param contractName Name of the contract to deploy
 * @param initializer Name of the initializer function to call
 * @param args Arguments to pass to the initializer
 * @returns Contract instance attached to the proxy address
 */
export async function deployUpgradeableContract(
  contractName: string,
  initializer: string,
  args: any[] = [],
) {
  // Deploy implementation
  const ImplFactory = await ethers.getContractFactory(contractName);
  const impl = await ImplFactory.deploy();

  // Get the ERC1967Proxy ABI and bytecode
  const proxyArtifactPath = require.resolve(
    "@openzeppelin/contracts/build/contracts/ERC1967Proxy.json"
  );
  const proxyArtifact = JSON.parse(readFileSync(proxyArtifactPath, "utf-8"));

  // Deploy proxy
  const signers = await ethers.getSigners();
  const proxyFactory = new ethers.ContractFactory(
    proxyArtifact.abi,
    proxyArtifact.bytecode,
    signers[0]
  );

  const initData = ImplFactory.interface.encodeFunctionData(initializer, args);
  const proxy = await proxyFactory.deploy(await impl.getAddress(), initData);

  // Return proxy attached to implementation ABI
  return ImplFactory.attach(await proxy.getAddress());
}
