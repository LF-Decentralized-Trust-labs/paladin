import PaladinClient, {
  PaladinVerifier,
  PentePrivacyGroup,
  PentePrivateContract,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import erc20Tracker from "../abis/NotoTrackerERC20.json";

export interface ERC20TrackerConstructorParams {
  name: string;
  symbol: string;
}

export const newERC20Tracker = async (
  pente: PentePrivacyGroup,
  from: PaladinVerifier,
  params: ERC20TrackerConstructorParams
) => {
  const address = await pente.deploy(
    erc20Tracker.abi,
    erc20Tracker.bytecode,
    from,
    params
  );
  return address ? new BondTracker(pente, address) : undefined;
};

export class BondTracker extends PentePrivateContract<ERC20TrackerConstructorParams> {
  constructor(
    protected evm: PentePrivacyGroup,
    public readonly address: string
  ) {
    super(evm, erc20Tracker.abi, address);
  }

  using(paladin: PaladinClient) {
    return new BondTracker(this.evm.using(paladin), this.address);
  }
}
