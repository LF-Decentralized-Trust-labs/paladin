import PaladinClient from "./paladin";

// Represents an in-flight transaction
export class TransactionFuture {
  constructor(
    protected paladin: PaladinClient,
    public readonly id: string | Promise<string>
  ) {}

  toString() {
    return this.id;
  }

  async waitForReceipt(waitMs = 5000, full = false) {
    return this.paladin.pollForReceipt(await this.id, waitMs, full);
  }
}
