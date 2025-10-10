export interface IRegistryEntry {
  registry: string;
  id: string;
  name: string;
  parentId?: string;
  blockNumber?: number;
  transactionIndex?: number;
  logIndex?: number;
  active?: boolean;
}

export interface IRegistryProperty {
  registry: string;
  entryId: string;
  name: string;
  value: string;
  blockNumber?: number;
  transactionIndex?: number;
  logIndex?: number;
  active?: boolean;
}

export interface IRegistryEntryWithProperties extends IRegistryEntry {
  properties: { [key: string]: string };
}

export type ActiveFilter = "active" | "inactive" | "any";
