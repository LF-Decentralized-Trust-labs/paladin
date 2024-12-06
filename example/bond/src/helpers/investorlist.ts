import PaladinClient, {
  PentePrivacyGroup,
  PentePrivateContract,
} from "paladin-sdk";
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

  addInvestor(from: string, params: AddInvestorParams) {
    return this.invoke(from, "addInvestor", params);
  }
}
