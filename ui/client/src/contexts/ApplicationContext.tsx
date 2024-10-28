import { Config } from "@/config";
import { useBidxQueries } from "@/queries/bidx";
import { useTransportQueries } from "@/queries/transport";
import { createContext } from "react";
import { ErrorDialog } from "../components/Dialogs/ErrorDialog";

interface IApplicationContext {
  lastBlockWithTransactions: number;
  nodeName: string;
}

export const ApplicationContext = createContext({} as IApplicationContext);

interface Props {
  children: JSX.Element;
}

export const ApplicationContextProvider = ({ children }: Props) => {
  const { useQueryIndexedTransactions } = useBidxQueries();
  const { useNodeName } = useTransportQueries();
  const { data: nodeName } = useNodeName();
  const { data: lastBlockWithTransactions, error } =
    useQueryIndexedTransactions(
      {
        limit: 1,
        sort: ["blockNumber DESC", "transactionIndex DESC"],
      },
      Config.UPDATE_FREQUENCY_MILLISECONDS
    );

  return (
    <ApplicationContext.Provider
      value={{
        lastBlockWithTransactions:
          lastBlockWithTransactions && lastBlockWithTransactions.length > 0
            ? lastBlockWithTransactions[0].blockNumber
            : 0,
        nodeName: nodeName ?? "",
      }}
    >
      {children}
      <ErrorDialog dialogOpen={!!error} message={error?.message ?? ""} />
    </ApplicationContext.Provider>
  );
};
