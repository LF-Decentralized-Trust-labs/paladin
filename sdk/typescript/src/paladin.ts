import axios, { AxiosError, AxiosInstance, AxiosRequestConfig } from "axios";
import { ethers, InterfaceAbi } from "ethers";
import {
  Algorithms,
  ISchema,
  IState,
  ITransactionReceiptListener,
  StateStatus,
  Verifiers,
} from "./interfaces";
import { IBlockchainEventListener } from "./interfaces/blockchainevent";
import { IEventWithData } from "./interfaces/blockindex";
import { Logger } from "./interfaces/logger";
import {
  JsonRpcResult,
  PaladinConfig,
  PaladinErrorHandler,
} from "./interfaces/paladin";
import {
  IPrivacyGroup,
  IPrivacyGroupEVMCall,
  IPrivacyGroupEVMTXInput,
  IPrivacyGroupInput,
} from "./interfaces/privacygroups";
import { IQuery } from "./interfaces/query";
import {
  IABIDecodedData,
  INotoDomainReceipt,
  IPenteDomainReceipt,
  IPreparedTransaction,
  IStoredABI,
  ITransaction,
  ITransactionCall,
  ITransactionInput,
  ITransactionReceipt,
  ITransactionStates,
} from "./interfaces/transaction";
import { PaladinVerifier } from "./verifier";

const POLL_INTERVAL_MS = 100;

export default class PaladinClient {
  protected http: AxiosInstance;
  private logger: Logger;
  private onError: PaladinErrorHandler;

  constructor(options: PaladinConfig) {
    this.http = axios.create({
      ...options.requestConfig,
      baseURL: options.url,
    });
    this.logger = options.logger ?? console;
    this.onError =
      options.onError ??
      ((method: string, err: AxiosError) => {
        this.logger.error(
          `JSON-RPC error from ${method} (${err.response?.status} ${err.response?.statusText})`,
          this.parseAxiosErrorMessage(err)
        );
      });
  }

  protected defaultHeaders() {
    return {
      Accept: "application/json",
      "Content-Type": "application/json",
    };
  }

  protected defaultPayload() {
    return {
      jsonrpc: "2.0",
      id: Date.now(),
    };
  }

  getVerifiers(...lookups: string[]) {
    return lookups.map((lookup) => new PaladinVerifier(this, lookup));
  }

  parseAxiosErrorMessage(err: any) {
    if (err instanceof AxiosError && err.response?.data?.error) {
      return err.response.data.error?.message || err.response.data.error;
    }
    return `${err}`;
  }

  private post<T>(method: string, params: any[], config?: AxiosRequestConfig) {
    const res = this.http.post<T>(
      "/",
      { ...this.defaultPayload(), method, params },
      { ...config, headers: this.defaultHeaders() }
    );
    res.catch((err: AxiosError) => this.onError(method, err));
    return res;
  }

  async pollForReceipt(txID: string, waitMs: number, full?: boolean) {
    for (let i = 0; i < waitMs; i += POLL_INTERVAL_MS) {
      var receipt = await this.getTransactionReceipt(txID, full);
      if (receipt != undefined) {
        return receipt;
      }
      await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
    }
    this.logger.error(`Failed while waiting for receipt: ${txID}`);
    return undefined;
  }

  async pollForPreparedTransaction(txID: string, waitMs: number) {
    for (let i = 0; i < waitMs; i += POLL_INTERVAL_MS) {
      var receipt = await this.getPreparedTransaction(txID);
      if (receipt != undefined) {
        return receipt;
      }
      await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
    }
    this.logger.error(`Failed while waiting for prepare: ${txID}`);
    return undefined;
  }

  async prepareTransaction(transaction: ITransactionInput) {
    const res = await this.post<JsonRpcResult<string>>(
      "ptx_prepareTransaction",
      [transaction],
      undefined
    );
    return res.data.result;
  }

  async sendTransaction(transaction: ITransactionInput) {
    const res = await this.post<JsonRpcResult<string>>(
      "ptx_sendTransaction",
      [transaction],
      undefined
    );
    return res.data.result;
  }

  async call(transaction: ITransactionCall) {
    const res = await this.post<JsonRpcResult<any>>("ptx_call", [transaction]);
    return res.data.result;
  }

  async queryTransactions(query: IQuery) {
    const res = await this.post<JsonRpcResult<ITransaction[]>>(
      "ptx_queryTransactions",
      [query]
    );
    return res.data.result;
  }

  async getTransaction(txID: string, full?: boolean) {
    const res = await this.post<JsonRpcResult<ITransaction>>(
      full ? "ptx_getTransactionFull" : "ptx_getTransaction",
      [txID],
      {
        validateStatus: (status) => status < 300 || status === 404,
      }
    );
    return res.status === 404 ? undefined : res.data.result;
  }

  async getPreparedTransaction(txID: string) {
    const res = await this.post<JsonRpcResult<IPreparedTransaction>>(
      "ptx_getPreparedTransaction",
      [txID],
      {
        validateStatus: (status) => status < 300 || status === 404,
      }
    );
    return res.status === 404 ? undefined : res.data.result;
  }

  async queryTransactionReceipts(query: IQuery) {
    const res = await this.post<JsonRpcResult<ITransactionReceipt[]>>(
      "ptx_queryTransactionReceipts",
      [query]
    );
    return res.data.result;
  }

  async getTransactionReceipt(txID: string, full?: boolean) {
    const res = await this.post<JsonRpcResult<ITransactionReceipt>>(
      full ? "ptx_getTransactionReceiptFull" : "ptx_getTransactionReceipt",
      [txID],
      {
        validateStatus: (status) => status < 300 || status === 404,
      }
    );
    return res.status === 404 ? undefined : res.data.result;
  }

  async getStateReceipt(txID: string) {
    const res = await this.post<JsonRpcResult<ITransactionStates>>(
      "ptx_getStateReceipt",
      [txID],
      {
        validateStatus: (status) => status < 300 || status === 404,
      }
    );
    return res.status === 404 ? undefined : res.data.result;
  }

  async getDomainReceipt(domain: string, txID: string) {
    const res = await this.post<
      JsonRpcResult<INotoDomainReceipt | IPenteDomainReceipt>
    >("ptx_getDomainReceipt", [domain, txID], {
      validateStatus: (status) => status < 300 || status === 404,
    });
    return res.status === 404 ? undefined : res.data.result;
  }

  async resolveVerifier(
    lookup: string,
    algorithm: Algorithms | string,
    verifierType: Verifiers | string
  ) {
    const res = await this.post<JsonRpcResult<string>>("ptx_resolveVerifier", [
      lookup,
      algorithm,
      verifierType,
    ]);
    return res.data.result;
  }

  async storeABI(abi: ethers.InterfaceAbi) {
    await this.post("ptx_storeABI", [abi]);
  }

  async getStoredABI(hash: string) {
    const res = await this.post<JsonRpcResult<IStoredABI>>("ptx_getStoredABI", [
      hash,
    ]);
    return res.data.result;
  }

  async decodeCall(callData: string, dataFormat: string) {
    const res = await this.post<JsonRpcResult<IABIDecodedData>>(
      "ptx_decodeCall",
      [callData, dataFormat]
    );
    return res.data.result;
  }

  async decodeEvent(topics: string[], data: string) {
    try {
      const res = await this.post<JsonRpcResult<IABIDecodedData>>(
        "ptx_decodeEvent",
        [topics, data, ""]
      );
      return res.data.result;
    } catch (err) {
      const parsed = this.parseAxiosErrorMessage(err);
      if (typeof parsed === "string" && parsed.indexOf("PD012229") >= 0) {
        return undefined;
      }
      throw err;
    }
  }

  async decodeTransactionEvents(
    transactionHash: string,
    abi: InterfaceAbi,
    resultFormat: string
  ) {
    const res = await this.post<JsonRpcResult<IEventWithData[]>>(
      "bidx_decodeTransactionEvents",
      [transactionHash, abi, resultFormat]
    );
    return res.data.result;
  }

  async listSchemas(domain: string) {
    const res = await this.post<JsonRpcResult<ISchema[]>>(
      "pstate_listSchemas",
      [domain]
    );
    return res.data.result;
  }

  async queryStates(
    domain: string,
    schema: string,
    query: IQuery,
    status: StateStatus
  ) {
    const res = await this.post<JsonRpcResult<IState[]>>("pstate_queryStates", [
      domain,
      schema,
      query,
      status,
    ]);
    return res.data.result;
  }

  async queryContractStates(
    domain: string,
    contractAddress: string,
    schema: string,
    query: IQuery,
    status: StateStatus
  ) {
    const res = await this.post<JsonRpcResult<IState[]>>(
      "pstate_queryContractStates",
      [domain, contractAddress, schema, query, status]
    );
    return res.data.result;
  }

  async createPrivacyGroup(pgroup: IPrivacyGroupInput) {
    const res = await this.post<JsonRpcResult<IPrivacyGroup>>(
      "pgroup_createGroup",
      [pgroup]
    );
    return res.data.result;
  }

  async getPrivacyGroupById(domainName: string, id: string) {
    const res = await this.post<JsonRpcResult<IPrivacyGroup>>(
      "pgroup_getGroupById",
      [domainName, id]
    );
    return res.data.result;
  }

  async getPrivacyGroupByAddress(address: string) {
    const res = await this.post<JsonRpcResult<IPrivacyGroup>>(
      "pgroup_getGroupByAddress",
      [address]
    );
    return res.data.result;
  }

  async sendPrivacyGroupTransaction(txi: IPrivacyGroupEVMTXInput) {
    const res = await this.post<JsonRpcResult<string>>(
      "pgroup_sendTransaction",
      [txi]
    );
    return res.data.result;
  }

  async callPrivacyGroup(txi: IPrivacyGroupEVMCall) {
    const res = await this.post<JsonRpcResult<any>>("pgroup_call", [txi]);
    return res.data.result;
  }

  async createReceiptListener(listener: ITransactionReceiptListener) {
    const res = await this.post<JsonRpcResult<boolean>>(
      "ptx_createReceiptListener",
      [listener]
    );
    return res.data.result;
  }

  async deleteReceiptListener(name: string) {
    const res = await this.post<JsonRpcResult<boolean>>(
      "ptx_deleteReceiptListener",
      [name]
    );
    return res.data.result;
  }

  async getReceiptListener(name: string) {
    const res = await this.post<JsonRpcResult<ITransactionReceiptListener>>(
      "ptx_getReceiptListener",
      [name],
      { validateStatus: (status) => status < 300 || status === 404 }
    );
    return res.status === 404 ? undefined : res.data.result;
  }

  async createBlockchainEventListener(listener: IBlockchainEventListener) {
    const res = await this.post<JsonRpcResult<boolean>>(
      "ptx_createBlockchainEventListener",
      [listener]
    );
    return res.data.result;
  }

  async deleteBlockchainEventListener(name: string) {
    const res = await this.post<JsonRpcResult<boolean>>(
      "ptx_deleteBlockchainEventListener",
      [name]
    );
    return res.data.result;
  }

  async getBlockchainEventListener(name: string) {
    const res = await this.post<JsonRpcResult<IBlockchainEventListener>>(
      "ptx_getBlockchainEventListener",
      [name],
      { validateStatus: (status) => status < 300 || status === 404 }
    );
    return res.status === 404 ? undefined : res.data.result;
  }
}
