import PaladinClient, {
  PaladinVerifier,
  PentePrivacyGroup,
  PentePrivateContract,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import investorList from "../abis/InvestorList.json";

export interface AddInvestorParams {
  addr: string;
}

export class InvestorList extends PentePrivateContract<{}> {
  constructor(
    protected evm: PentePrivacyGroup,
    public readonly address: string
  ) {
    super(evm, investorList.abi, address);
  }

  using(paladin: PaladinClient) {
    return new InvestorList(this.evm.using(paladin), this.address);
  }

  addInvestor(from: PaladinVerifier, params: AddInvestorParams) {
    return this.sendTransaction({
      from: from.lookup,
      function: "addInvestor",
      data: params,
    });
  }
}
