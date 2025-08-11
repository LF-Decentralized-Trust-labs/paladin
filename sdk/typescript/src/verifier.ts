import { Algorithms, Verifiers } from "./interfaces";
import PaladinClient from "./paladin";

export class PaladinVerifier {
  constructor(private paladin: PaladinClient, public readonly lookup: string) {}

  toString() {
    return this.lookup;
  }

  split() {
    const [identity, node] = this.lookup.split("@");
    return { 0: identity, 1: node, identity, node };
  }

  resolve(algorithm: Algorithms, verifierType: Verifiers) {
    return this.paladin.ptx.resolveVerifier(
      this.lookup,
      algorithm,
      verifierType
    );
  }

  address() {
    return this.resolve(Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);
  }
}
