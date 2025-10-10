export interface IWalletInfo {
  name: string;
  type: string;
  description: string;
}

export interface IKeyMappingAndVerifier {
  identifier: string;
  keyHandle: string;
  path: {
    index: number;
    name: string;
  }[];
  verifier: {
    verifier: string;
    type: string;
    algorithm: string;
  };
  wallet: string;
}

export interface IEthAddress {
  address: string;
}

export interface IKeyQueryEntry {
  identifier: string;
  keyHandle: string;
  wallet: string;
  verifiers: {
    verifier: string;
    type: string;
    algorithm: string;
  }[];
}
