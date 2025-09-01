import PaladinClient, {
  PaladinVerifier,
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import atomJson from "../abis/Atom.json";
import atomFactoryJson from "../abis/AtomFactory.json";

export interface AtomOperation {
  contractAddress: string;
  callData: string;
}

export const newAtomFactory = async (
  paladin: PaladinClient,
  from: PaladinVerifier
) => {
  const txID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: atomFactoryJson.abi,
    bytecode: atomFactoryJson.bytecode,
    function: "",
    from: from.lookup,
    data: {},
  });
  const receipt = await paladin.pollForReceipt(txID, 10000);
  return receipt?.contractAddress
    ? new AtomFactory(paladin, receipt.contractAddress)
    : undefined;
};

export class AtomFactory {
  constructor(
    protected paladin: PaladinClient,
    public readonly address: string
  ) {}

  using(paladin: PaladinClient) {
    return new AtomFactory(paladin, this.address);
  }

  async create(from: PaladinVerifier, operations: AtomOperation[]) {
    const txID = await this.paladin.ptx.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: atomFactoryJson.abi,
      function: "create",
      from: from.lookup,
      to: this.address,
      data: { operations },
    });
    const receipt = await this.paladin.pollForReceipt(txID, 10000);
    if (receipt) {
      const events = await this.paladin.bidx.decodeTransactionEvents(
        receipt.transactionHash,
        atomFactoryJson.abi,
        ""
      );
      const deployedEvent = events.find((ev) =>
        ev.soliditySignature.startsWith("event AtomDeployed")
      );
      const atomAddress = deployedEvent?.data.addr;
      return atomAddress ? new Atom(this.paladin, atomAddress) : undefined;
    }
    return undefined;
  }
}

export class Atom {
  constructor(
    protected paladin: PaladinClient,
    public readonly address: string
  ) {}

  using(paladin: PaladinClient) {
    return new Atom(paladin, this.address);
  }

  async execute(from: PaladinVerifier) {
    const txID = await this.paladin.ptx.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: atomJson.abi,
      function: "execute",
      from: from.lookup,
      to: this.address,
      data: {},
    });
    return this.paladin.pollForReceipt(txID, 10000);
  }
}
