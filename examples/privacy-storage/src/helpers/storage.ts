import PaladinClient, {
  PaladinVerifier,
  PentePrivacyGroup,
  PentePrivateContract,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import storage from "../abis/Storage.json";

export const newPrivateStorage = async (
  pente: PentePrivacyGroup,
  from: PaladinVerifier,
) => {
  const address = await pente.deploy({
    abi: storage.abi,
    bytecode: storage.bytecode,
    from: from.lookup,
  }).waitForDeploy();
  return address ? new PrivateStorage(pente, address) : undefined;
};

export class PrivateStorage extends PentePrivateContract<{}> {
  constructor(
    protected evm: PentePrivacyGroup,
    public readonly address: string
  ) {
    super(evm, storage.abi, address);
  }

  using(paladin: PaladinClient) {
    return new PrivateStorage(this.evm.using(paladin), this.address);
  }
}
