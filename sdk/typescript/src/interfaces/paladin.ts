import { AxiosError, AxiosRequestConfig } from "axios";
import { Logger } from "./logger";

export interface PaladinConfig {
  url: string;
  requestConfig?: AxiosRequestConfig;
  logger?: Logger;
  onError?: PaladinErrorHandler;
}

export interface PaladinErrorHandler {
  (method: string, err: AxiosError): void | Promise<void>;
}

export interface JsonRpcResult<T> {
  result: T;
}
