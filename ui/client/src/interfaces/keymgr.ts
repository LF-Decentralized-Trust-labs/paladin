export interface WalletInfo {
  name: string;
  keySelector: string;
}

export interface KeyMapping {
  identifier: string;
  wallet: string;
  keyHandle: string;
}

export interface KeyMappingWithPath extends KeyMapping {
  path: KeyPathSegment[];
}

export interface KeyMappingAndVerifier extends KeyMappingWithPath {
  verifier: KeyVerifier;
}

export interface KeyVerifierWithKeyRef extends KeyVerifier {
  keyIdentifier: string;
}

export interface KeyVerifier {
  verifier: string;
  type: string;
  algorithm: string;
}

export interface KeyPathSegment {
  name: string;
  index: number;
}
