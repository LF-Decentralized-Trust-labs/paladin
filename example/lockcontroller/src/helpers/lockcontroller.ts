import PaladinClient, {
  PaladinVerifier,
  TransactionType,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import lockControllerJson from "../abis/LockController.json";

export const newLockController = async (
  paladin: PaladinClient,
  from: PaladinVerifier
) => {
  const txID = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: lockControllerJson.abi,
    bytecode: lockControllerJson.bytecode,
    function: "",
    from: from.lookup,
    data: {},
  });
  const receipt = await paladin.pollForReceipt(txID, 10000);
  return receipt?.contractAddress
    ? new LockController(paladin, receipt.contractAddress)
    : undefined;
};

export class LockController {
  constructor(
    protected paladin: PaladinClient,
    public readonly address: string
  ) {}

  using(paladin: PaladinClient) {
    return new LockController(paladin, this.address);
  }

  getToken(from: PaladinVerifier, lockId: string) {
    return this.paladin.call({
      type: TransactionType.PUBLIC,
      abi: lockControllerJson.abi,
      function: "locks",
      from: from.lookup,
      to: this.address,
      data: [lockId],
    });
  }
}
