import { ethers } from "ethers";

export interface IBlockchainEventListener {
  name: string;
  sources: IBlockchainEventListenerSource[];
  options?: IBlockchainEventListenerOptions;
}

export interface IBlockchainEventListenerOptions {
  batchSize?: number;
  batchTimeout?: string;
  fromBlock?: string;
}

export interface IBlockchainEventListenerSource {
  abi: ethers.JsonFragment[];
  address?: string;
}
