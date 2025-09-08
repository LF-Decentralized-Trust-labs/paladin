import * as uuid from "uuid";
import { IStateBase, IStateEncoded } from "./interfaces";

export function encodeHex(data: string) {
  return "0x" + Buffer.from(data, "utf8").toString("hex");
}

export function decodeHex(data: string) {
  return Buffer.from(data.slice(2), "hex").toString("utf8");
}

export function newTransactionId() {
  return encodeHex(uuid.v4()) + "00000000000000000000000000000000";
}

export const encodeStates = (states: IStateBase[]): IStateEncoded[] => {
  return states.map((state) => ({
    id: state.id,
    domain: state.domain,
    schema: state.schema,
    contractAddress: state.contractAddress,
    data: encodeHex(JSON.stringify(state.data)),
  }));
};
