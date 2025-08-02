import axios, { AxiosError, AxiosInstance, AxiosRequestConfig } from "axios";
import { ethers, InterfaceAbi } from "ethers";
import {
  ActiveFilter,
  Algorithms,
  IABIDecodedData,
  IBlockchainEventListener,
  IEthAddress,
  IEventWithData,
  IKeyMappingAndVerifier,
  IKeyQueryEntry,
  INotoDomainReceipt,
  IPenteDomainReceipt,
  IPreparedTransaction,
  IPrivacyGroup,
  IPrivacyGroupEVMCall,
  IPrivacyGroupEVMTXInput,
  IPrivacyGroupInput,
  IPrivacyGroupMessageInput,
  IPrivacyGroupMessageListener,
  IQuery,
  IRegistryEntry,
  IRegistryEntryWithProperties,
  IRegistryProperty,
  ISchema,
  IState,
  IStoredABI,
  ITransaction,
  ITransactionCall,
  ITransactionInput,
  ITransactionReceipt,
  ITransactionReceiptListener,
  ITransactionStates,
  IWalletInfo,
  JsonRpcResult,
  Logger,
  PaladinConfig,
  PaladinErrorHandler,
  StateStatus,
  Verifiers,
} from "./interfaces";
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
      var receipt = full 
        ? await this.ptx.getTransactionReceiptFull(txID)
        : await this.ptx.getTransactionReceipt(txID);
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
      var receipt = await this.ptx.getPreparedTransaction(txID);
      if (receipt != undefined) {
        return receipt;
      }
      await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
    }
    this.logger.error(`Failed while waiting for prepare: ${txID}`);
    return undefined;
  }

  /**
   * @deprecated Use ptx.prepareTransaction instead
   */
  async prepareTransaction(transaction: ITransactionInput) {
    return this.ptx.prepareTransaction(transaction);
  }

  /**
   * @deprecated Use ptx.sendTransaction instead
   */
  async sendTransaction(transaction: ITransactionInput) {
    return this.ptx.sendTransaction(transaction);
  }

  /**
   * @deprecated Use ptx.call instead
   */
  async call(transaction: ITransactionCall) {
    return this.ptx.call(transaction);
  }

  /**
   * @deprecated Use ptx.queryTransactions instead
   */
  async queryTransactions(query: IQuery) {
    return this.ptx.queryTransactions(query);
  }

  /**
   * @deprecated Use ptx.getTransaction or ptx.getTransactionFull instead
   */
  async getTransaction(txID: string, full?: boolean) {
    return full
      ? this.ptx.getTransactionFull(txID)
      : this.ptx.getTransaction(txID);
  }

  /**
   * @deprecated Use ptx.getPreparedTransaction instead
   */
  async getPreparedTransaction(txID: string) {
    return this.ptx.getPreparedTransaction(txID);
  }

  /**
   * @deprecated Use ptx.queryTransactionReceipts instead
   */
  async queryTransactionReceipts(query: IQuery) {
    return this.ptx.queryTransactionReceipts(query);
  }

  /**
   * @deprecated Use ptx.getTransactionReceipt or ptx.getTransactionReceiptFull instead
   */
  async getTransactionReceipt(txID: string, full?: boolean) {
    return full
      ? this.ptx.getTransactionReceiptFull(txID)
      : this.ptx.getTransactionReceipt(txID);
  }

  /**
   * @deprecated Use ptx.getStateReceipt instead
   */
  async getStateReceipt(txID: string) {
    return this.ptx.getStateReceipt(txID);
  }

  /**
   * @deprecated Use ptx.getDomainReceipt instead
   */
  async getDomainReceipt(domain: string, txID: string) {
    return this.ptx.getDomainReceipt(domain, txID);
  }

  /**
   * @deprecated Use ptx.resolveVerifier instead
   */
  async resolveVerifier(
    lookup: string,
    algorithm: Algorithms | string,
    verifierType: Verifiers | string
  ) {
    return this.ptx.resolveVerifier(lookup, algorithm, verifierType);
  }

  /**
   * @deprecated Use ptx.storeABI instead
   */
  async storeABI(abi: ethers.InterfaceAbi) {
    return this.ptx.storeABI(abi);
  }

  /**
   * @deprecated Use ptx.getStoredABI instead
   */
  async getStoredABI(hash: string) {
    return this.ptx.getStoredABI(hash);
  }

  /**
   * @deprecated Use ptx.decodeCall instead
   */
  async decodeCall(callData: string, dataFormat: string) {
    return this.ptx.decodeCall(callData, dataFormat);
  }

  /**
   * @deprecated Use ptx.decodeEvent instead
   */
  async decodeEvent(topics: string[], data: string) {
    return this.ptx.decodeEvent(topics, data);
  }

  /**
   * @deprecated Use ptx.decodeError instead
   */
  async decodeError(revertError: string, dataFormat: string) {
    return this.ptx.decodeError(revertError, dataFormat);
  }

  /**
   * @deprecated Use bidx.decodeTransactionEvents instead
   */
  async decodeTransactionEvents(
    transactionHash: string,
    abi: InterfaceAbi,
    resultFormat: string
  ) {
    return this.bidx.decodeTransactionEvents(
      transactionHash,
      abi,
      resultFormat
    );
  }

  /**
   * @deprecated Use pstate.listSchemas instead
   */
  async listSchemas(domain: string) {
    return this.pstate.listSchemas(domain);
  }

  /**
   * @deprecated Use pstate.queryStates instead
   */
  async queryStates(
    domain: string,
    schema: string,
    query: IQuery,
    status: StateStatus
  ) {
    return this.pstate.queryStates(domain, schema, query, status);
  }

  /**
   * @deprecated Use pstate.queryContractStates instead
   */
  async queryContractStates(
    domain: string,
    contractAddress: string,
    schema: string,
    query: IQuery,
    status: StateStatus
  ) {
    return this.pstate.queryContractStates(
      domain,
      contractAddress,
      schema,
      query,
      status
    );
  }

  /**
   * @deprecated Use pgroup.createGroup instead
   */
  async createPrivacyGroup(pgroup: IPrivacyGroupInput) {
    return this.pgroup.createGroup(pgroup);
  }

  /**
   * @deprecated Use pgroup.getGroupById instead
   */
  async getPrivacyGroupById(domainName: string, id: string) {
    return this.pgroup.getGroupById(domainName, id);
  }

  /**
   * @deprecated Use pgroup.getGroupByAddress instead
   */
  async getPrivacyGroupByAddress(address: string) {
    return this.pgroup.getGroupByAddress(address);
  }

  /**
   * @deprecated Use pgroup.sendTransaction instead
   */
  async sendPrivacyGroupTransaction(txi: IPrivacyGroupEVMTXInput) {
    return this.pgroup.sendTransaction(txi);
  }

  /**
   * @deprecated Use pgroup.call instead
   */
  async callPrivacyGroup(txi: IPrivacyGroupEVMCall) {
    return this.pgroup.call(txi);
  }

  /**
   * @deprecated Use ptx.createReceiptListener instead
   */
  async createReceiptListener(listener: ITransactionReceiptListener) {
    return this.ptx.createReceiptListener(listener);
  }

  /**
   * @deprecated Use ptx.deleteReceiptListener instead
   */
  async deleteReceiptListener(name: string) {
    return this.ptx.deleteReceiptListener(name);
  }

  /**
   * @deprecated Use ptx.getReceiptListener instead
   */
  async getReceiptListener(name: string) {
    return this.ptx.getReceiptListener(name);
  }

  /**
   * @deprecated Use ptx.createBlockchainEventListener instead
   */
  async createBlockchainEventListener(listener: IBlockchainEventListener) {
    return this.ptx.createBlockchainEventListener(listener);
  }

  /**
   * @deprecated Use ptx.deleteBlockchainEventListener instead
   */
  async deleteBlockchainEventListener(name: string) {
    return this.ptx.deleteBlockchainEventListener(name);
  }

  /**
   * @deprecated Use ptx.getBlockchainEventListener instead
   */
  async getBlockchainEventListener(name: string) {
    return this.ptx.getBlockchainEventListener(name);
  }

  keymgr = {
    wallets: async () => {
      const res = await this.post<JsonRpcResult<IWalletInfo[]>>(
        "keymgr_wallets",
        []
      );
      return res.data.result;
    },

    resolveKey: async (
      identifier: string,
      algorithm: string,
      verifierType: string
    ) => {
      const res = await this.post<JsonRpcResult<IKeyMappingAndVerifier>>(
        "keymgr_resolveKey",
        [identifier, algorithm, verifierType]
      );
      return res.data.result;
    },

    resolveEthAddress: async (identifier: string) => {
      const res = await this.post<JsonRpcResult<IEthAddress>>(
        "keymgr_resolveEthAddress",
        [identifier]
      );
      return res.data.result;
    },

    reverseKeyLookup: async (
      algorithm: string,
      verifierType: string,
      verifier: string
    ) => {
      const res = await this.post<JsonRpcResult<IKeyMappingAndVerifier>>(
        "keymgr_reverseKeyLookup",
        [algorithm, verifierType, verifier]
      );
      return res.data.result;
    },

    queryKeys: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<IKeyQueryEntry[]>>(
        "keymgr_queryKeys",
        [query]
      );
      return res.data.result;
    },
  };

  ptx = {
    sendTransaction: async (transaction: ITransactionInput) => {
      const res = await this.post<JsonRpcResult<string>>(
        "ptx_sendTransaction",
        [transaction],
        undefined
      );
      return res.data.result;
    },

    sendTransactions: async (transactions: ITransactionInput[]) => {
      const res = await this.post<JsonRpcResult<string[]>>(
        "ptx_sendTransactions",
        [transactions],
        undefined
      );
      return res.data.result;
    },

    prepareTransaction: async (transaction: ITransactionInput) => {
      const res = await this.post<JsonRpcResult<string>>(
        "ptx_prepareTransaction",
        [transaction],
        undefined
      );
      return res.data.result;
    },

    prepareTransactions: async (transactions: ITransactionInput[]) => {
      const res = await this.post<JsonRpcResult<string[]>>(
        "ptx_prepareTransactions",
        [transactions],
        undefined
      );
      return res.data.result;
    },

    updateTransaction: async (id: string, transaction: ITransactionInput) => {
      const res = await this.post<JsonRpcResult<string>>(
        "ptx_updateTransaction",
        [id, transaction],
        undefined
      );
      return res.data.result;
    },

    call: async (transaction: ITransactionCall) => {
      const res = await this.post<JsonRpcResult<any>>("ptx_call", [
        transaction,
      ]);
      return res.data.result;
    },

    getTransaction: async (txID: string) => {
      const res = await this.post<JsonRpcResult<ITransaction>>(
        "ptx_getTransaction",
        [txID],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getTransactionFull: async (txID: string) => {
      const res = await this.post<JsonRpcResult<ITransaction>>(
        "ptx_getTransactionFull",
        [txID],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getTransactionByIdempotencyKey: async (idempotencyKey: string) => {
      const res = await this.post<JsonRpcResult<ITransaction>>(
        "ptx_getTransactionByIdempotencyKey",
        [idempotencyKey],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    queryTransactions: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<ITransaction[]>>(
        "ptx_queryTransactions",
        [query]
      );
      return res.data.result;
    },

    queryTransactionsFull: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<ITransaction[]>>(
        "ptx_queryTransactionsFull",
        [query]
      );
      return res.data.result;
    },

    queryPendingTransactions: async (query: IQuery, full?: boolean) => {
      const res = await this.post<JsonRpcResult<ITransaction[]>>(
        "ptx_queryPendingTransactions",
        [query, full]
      );
      return res.data.result;
    },

    getTransactionReceipt: async (txID: string) => {
      const res = await this.post<JsonRpcResult<ITransactionReceipt>>(
        "ptx_getTransactionReceipt",
        [txID],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getTransactionReceiptFull: async (txID: string) => {
      const res = await this.post<JsonRpcResult<ITransactionReceipt>>(
        "ptx_getTransactionReceiptFull",
        [txID],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getDomainReceipt: async (domain: string, txID: string) => {
      const res = await this.post<
        JsonRpcResult<INotoDomainReceipt | IPenteDomainReceipt>
      >("ptx_getDomainReceipt", [domain, txID], {
        validateStatus: (status) => status < 300 || status === 404,
      });
      return res.status === 404 ? undefined : res.data.result;
    },

    getStateReceipt: async (txID: string) => {
      const res = await this.post<JsonRpcResult<ITransactionStates>>(
        "ptx_getStateReceipt",
        [txID],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    queryTransactionReceipts: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<ITransactionReceipt[]>>(
        "ptx_queryTransactionReceipts",
        [query]
      );
      return res.data.result;
    },

    getTransactionDependencies: async (txID: string) => {
      const res = await this.post<JsonRpcResult<string[]>>(
        "ptx_getTransactionDependencies",
        [txID],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    queryPublicTransactions: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "ptx_queryPublicTransactions",
        [query]
      );
      return res.data.result;
    },

    queryPendingPublicTransactions: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "ptx_queryPendingPublicTransactions",
        [query]
      );
      return res.data.result;
    },

    getPublicTransactionByNonce: async (from: string, nonce: number) => {
      const res = await this.post<JsonRpcResult<any>>(
        "ptx_getPublicTransactionByNonce",
        [from, nonce],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getPublicTransactionByHash: async (hash: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "ptx_getPublicTransactionByHash",
        [hash],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getPreparedTransaction: async (txID: string) => {
      const res = await this.post<JsonRpcResult<IPreparedTransaction>>(
        "ptx_getPreparedTransaction",
        [txID],
        {
          validateStatus: (status) => status < 300 || status === 404,
        }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    queryPreparedTransactions: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<IPreparedTransaction[]>>(
        "ptx_queryPreparedTransactions",
        [query]
      );
      return res.data.result;
    },

    storeABI: async (abi: ethers.InterfaceAbi) => {
      await this.post("ptx_storeABI", [abi]);
    },

    getStoredABI: async (hash: string) => {
      const res = await this.post<JsonRpcResult<IStoredABI>>(
        "ptx_getStoredABI",
        [hash]
      );
      return res.data.result;
    },

    queryStoredABIs: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<IStoredABI[]>>(
        "ptx_queryStoredABIs",
        [query]
      );
      return res.data.result;
    },

    decodeCall: async (callData: string, dataFormat: string) => {
      const res = await this.post<JsonRpcResult<IABIDecodedData>>(
        "ptx_decodeCall",
        [callData, dataFormat]
      );
      return res.data.result;
    },

    decodeEvent: async (topics: string[], data: string) => {
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
    },

    decodeError: async (revertError: string, dataFormat: string) => {
      const res = await this.post<JsonRpcResult<IABIDecodedData>>(
        "ptx_decodeError",
        [revertError, dataFormat]
      );
      return res.data.result;
    },

    resolveVerifier: async (
      lookup: string,
      algorithm: Algorithms | string,
      verifierType: Verifiers | string
    ) => {
      const res = await this.post<JsonRpcResult<string>>(
        "ptx_resolveVerifier",
        [lookup, algorithm, verifierType]
      );
      return res.data.result;
    },

    createReceiptListener: async (listener: ITransactionReceiptListener) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "ptx_createReceiptListener",
        [listener]
      );
      return res.data.result;
    },

    queryReceiptListeners: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<ITransactionReceiptListener[]>>(
        "ptx_queryReceiptListeners",
        [query]
      );
      return res.data.result;
    },

    getReceiptListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<ITransactionReceiptListener>>(
        "ptx_getReceiptListener",
        [name],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    startReceiptListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "ptx_startReceiptListener",
        [name]
      );
      return res.data.result;
    },

    stopReceiptListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "ptx_stopReceiptListener",
        [name]
      );
      return res.data.result;
    },

    deleteReceiptListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "ptx_deleteReceiptListener",
        [name]
      );
      return res.data.result;
    },

    createBlockchainEventListener: async (
      listener: IBlockchainEventListener
    ) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "ptx_createBlockchainEventListener",
        [listener]
      );
      return res.data.result;
    },

    queryBlockchainEventListeners: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<IBlockchainEventListener[]>>(
        "ptx_queryBlockchainEventListeners",
        [query]
      );
      return res.data.result;
    },

    getBlockchainEventListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<IBlockchainEventListener>>(
        "ptx_getBlockchainEventListener",
        [name],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    startBlockchainEventListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "ptx_startBlockchainEventListener",
        [name]
      );
      return res.data.result;
    },

    stopBlockchainEventListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "ptx_stopBlockchainEventListener",
        [name]
      );
      return res.data.result;
    },

    deleteBlockchainEventListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "ptx_deleteBlockchainEventListener",
        [name]
      );
      return res.data.result;
    },

    getBlockchainEventListenerStatus: async (name: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "ptx_getBlockchainEventListenerStatus",
        [name],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },
  };

  pstate = {
    listSchemas: async (domain: string) => {
      const res = await this.post<JsonRpcResult<ISchema[]>>(
        "pstate_listSchemas",
        [domain]
      );
      return res.data.result;
    },

    getSchemaById: async (domain: string, schemaId: string) => {
      const res = await this.post<JsonRpcResult<ISchema>>(
        "pstate_getSchemaById",
        [domain, schemaId],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    storeState: async (
      domain: string,
      contractAddress: string,
      schema: string,
      data: any
    ) => {
      const res = await this.post<JsonRpcResult<IState>>("pstate_storeState", [
        domain,
        contractAddress,
        schema,
        data,
      ]);
      return res.data.result;
    },

    queryStates: async (
      domain: string,
      schema: string,
      query: IQuery,
      status: StateStatus
    ) => {
      const res = await this.post<JsonRpcResult<IState[]>>(
        "pstate_queryStates",
        [domain, schema, query, status]
      );
      return res.data.result;
    },

    queryContractStates: async (
      domain: string,
      contractAddress: string,
      schema: string,
      query: IQuery,
      status: StateStatus
    ) => {
      const res = await this.post<JsonRpcResult<IState[]>>(
        "pstate_queryContractStates",
        [domain, contractAddress, schema, query, status]
      );
      return res.data.result;
    },

    queryNullifiers: async (
      domain: string,
      schema: string,
      query: IQuery,
      status: StateStatus
    ) => {
      const res = await this.post<JsonRpcResult<IState[]>>(
        "pstate_queryNullifiers",
        [domain, schema, query, status]
      );
      return res.data.result;
    },

    queryContractNullifiers: async (
      domain: string,
      contractAddress: string,
      schema: string,
      query: IQuery,
      status: StateStatus
    ) => {
      const res = await this.post<JsonRpcResult<IState[]>>(
        "pstate_queryContractNullifiers",
        [domain, contractAddress, schema, query, status]
      );
      return res.data.result;
    },
  };

  pgroup = {
    createGroup: async (pgroup: IPrivacyGroupInput) => {
      const res = await this.post<JsonRpcResult<IPrivacyGroup>>(
        "pgroup_createGroup",
        [pgroup]
      );
      return res.data.result;
    },

    getGroupById: async (domainName: string, id: string) => {
      const res = await this.post<JsonRpcResult<IPrivacyGroup>>(
        "pgroup_getGroupById",
        [domainName, id]
      );
      return res.data.result;
    },

    getGroupByAddress: async (address: string) => {
      const res = await this.post<JsonRpcResult<IPrivacyGroup>>(
        "pgroup_getGroupByAddress",
        [address]
      );
      return res.data.result;
    },

    queryGroups: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<IPrivacyGroup[]>>(
        "pgroup_queryGroups",
        [query]
      );
      return res.data.result;
    },

    queryGroupsWithMember: async (member: string, query: IQuery) => {
      const res = await this.post<JsonRpcResult<IPrivacyGroup[]>>(
        "pgroup_queryGroupsWithMember",
        [member, query]
      );
      return res.data.result;
    },

    sendTransaction: async (txi: IPrivacyGroupEVMTXInput) => {
      const res = await this.post<JsonRpcResult<string>>(
        "pgroup_sendTransaction",
        [txi]
      );
      return res.data.result;
    },

    call: async (txi: IPrivacyGroupEVMCall) => {
      const res = await this.post<JsonRpcResult<any>>("pgroup_call", [txi]);
      return res.data.result;
    },

    sendMessage: async (msg: IPrivacyGroupMessageInput) => {
      const res = await this.post<JsonRpcResult<string>>("pgroup_sendMessage", [
        msg,
      ]);
      return res.data.result;
    },

    getMessageById: async (id: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "pgroup_getMessageById",
        [id],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    queryMessages: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "pgroup_queryMessages",
        [query]
      );
      return res.data.result;
    },

    createMessageListener: async (listener: IPrivacyGroupMessageListener) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "pgroup_createMessageListener",
        [listener]
      );
      return res.data.result;
    },

    queryMessageListeners: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<IPrivacyGroupMessageListener[]>>(
        "pgroup_queryMessageListeners",
        [query]
      );
      return res.data.result;
    },

    getMessageListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<IPrivacyGroupMessageListener>>(
        "pgroup_getMessageListener",
        [name],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    startMessageListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "pgroup_startMessageListener",
        [name]
      );
      return res.data.result;
    },

    stopMessageListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "pgroup_stopMessageListener",
        [name]
      );
      return res.data.result;
    },

    deleteMessageListener: async (name: string) => {
      const res = await this.post<JsonRpcResult<boolean>>(
        "pgroup_deleteMessageListener",
        [name]
      );
      return res.data.result;
    },
  };

  transport = {
    nodeName: async () => {
      const res = await this.post<JsonRpcResult<string>>(
        "transport_nodeName",
        []
      );
      return res.data.result;
    },

    localTransports: async () => {
      const res = await this.post<JsonRpcResult<string[]>>(
        "transport_localTransports",
        []
      );
      return res.data.result;
    },

    localTransportDetails: async (transportName: string) => {
      const res = await this.post<JsonRpcResult<string>>(
        "transport_localTransportDetails",
        [transportName]
      );
      return res.data.result;
    },

    peers: async () => {
      const res = await this.post<JsonRpcResult<any[]>>("transport_peers", []);
      return res.data.result;
    },

    peerInfo: async (nodeName: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "transport_peerInfo",
        [nodeName],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    queryReliableMessages: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "transport_queryReliableMessages",
        [query]
      );
      return res.data.result;
    },

    queryReliableMessageAcks: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "transport_queryReliableMessageAcks",
        [query]
      );
      return res.data.result;
    },
  };

  domain = {
    listDomains: async () => {
      const res = await this.post<JsonRpcResult<string[]>>(
        "domain_listDomains",
        []
      );
      return res.data.result;
    },

    getDomain: async (name: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "domain_getDomain",
        [name],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getDomainByAddress: async (address: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "domain_getDomainByAddress",
        [address],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    querySmartContracts: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "domain_querySmartContracts",
        [query]
      );
      return res.data.result;
    },

    getSmartContractByAddress: async (address: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "domain_getSmartContractByAddress",
        [address],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },
  };

  bidx = {
    getBlockByNumber: async (number: number) => {
      const res = await this.post<JsonRpcResult<any>>(
        "bidx_getBlockByNumber",
        [number],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getTransactionByHash: async (hash: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "bidx_getTransactionByHash",
        [hash],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getTransactionByNonce: async (from: string, nonce: number) => {
      const res = await this.post<JsonRpcResult<any>>(
        "bidx_getTransactionByNonce",
        [from, nonce],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },

    getBlockTransactionsByNumber: async (blockNumber: number) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "bidx_getBlockTransactionsByNumber",
        [blockNumber]
      );
      return res.data.result;
    },

    getTransactionEventsByHash: async (hash: string) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "bidx_getTransactionEventsByHash",
        [hash]
      );
      return res.data.result;
    },

    queryIndexedBlocks: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "bidx_queryIndexedBlocks",
        [query]
      );
      return res.data.result;
    },

    queryIndexedTransactions: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "bidx_queryIndexedTransactions",
        [query]
      );
      return res.data.result;
    },

    queryIndexedEvents: async (query: IQuery) => {
      const res = await this.post<JsonRpcResult<any[]>>(
        "bidx_queryIndexedEvents",
        [query]
      );
      return res.data.result;
    },

    getConfirmedBlockHeight: async () => {
      const res = await this.post<JsonRpcResult<number>>(
        "bidx_getConfirmedBlockHeight",
        []
      );
      return res.data.result;
    },

    decodeTransactionEvents: async (
      transactionHash: string,
      abi: InterfaceAbi,
      resultFormat: string
    ) => {
      const res = await this.post<JsonRpcResult<IEventWithData[]>>(
        "bidx_decodeTransactionEvents",
        [transactionHash, abi, resultFormat]
      );
      return res.data.result;
    },
  };

  debug = {
    getTransactionStatus: async (txID: string) => {
      const res = await this.post<JsonRpcResult<any>>(
        "debug_getTransactionStatus",
        [txID],
        { validateStatus: (status) => status < 300 || status === 404 }
      );
      return res.status === 404 ? undefined : res.data.result;
    },
  };

  reg = {
    registries: async () => {
      const res = await this.post<JsonRpcResult<string[]>>(
        "reg_registries",
        []
      );
      return res.data.result;
    },

    queryEntries: async (
      registryName: string,
      query: IQuery,
      activeFilter: ActiveFilter
    ) => {
      const res = await this.post<JsonRpcResult<IRegistryEntry[]>>(
        "reg_queryEntries",
        [registryName, query, activeFilter]
      );
      return res.data.result;
    },

    queryEntriesWithProps: async (
      registryName: string,
      query: IQuery,
      activeFilter: ActiveFilter
    ) => {
      const res = await this.post<
        JsonRpcResult<IRegistryEntryWithProperties[]>
      >("reg_queryEntriesWithProps", [registryName, query, activeFilter]);
      return res.data.result;
    },

    getEntryProperties: async (
      registryName: string,
      entryId: string,
      activeFilter: ActiveFilter
    ) => {
      const res = await this.post<JsonRpcResult<IRegistryProperty[]>>(
        "reg_getEntryProperties",
        [registryName, entryId, activeFilter]
      );
      return res.data.result;
    },
  };
}
