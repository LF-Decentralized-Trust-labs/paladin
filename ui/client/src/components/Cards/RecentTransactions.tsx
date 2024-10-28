import { Config } from "@/config";
import { useBidxQueries } from "@/queries/bidx";
import { usePtxQueries } from "@/queries/ptx";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { ScrollArea } from "../ui/scroll-area";
import { TxCard } from "./TxCard";

export const RecentTransactions = () => {
  const { t } = useTranslation();

  const { useQueryIndexedTransactions } = useBidxQueries();
  const { useQueryTransactionReceipts, useQueryTransactionsFull } =
    usePtxQueries();

  const { data: transactions, isLoading: isTransactionsLoading } =
    useQueryIndexedTransactions({
      limit: Config.TRANSACTION_QUERY_LIMIT,
      sort: ["blockNumber DESC", "transactionIndex DESC"],
    });

  const { data: transactionReceipts } = useQueryTransactionReceipts({
    limit: Config.TRANSACTION_QUERY_LIMIT,
    in: [
      {
        field: "transactionHash",
        values:
          transactions?.map((transaction) => transaction.hash.substring(2)) ??
          [],
      },
    ],
  });

  const { data: paladinTransactions } = useQueryTransactionsFull({
    limit: Config.TRANSACTION_QUERY_LIMIT,
    in: [
      {
        field: "id",
        values: transactionReceipts?.map((transaction) => transaction.id) ?? [],
      },
    ],
  });

  return (
    <Card className="border rounded-md">
      <CardHeader className="py-4 bg-background">
        <CardTitle className="flex justify-between  text-xl text-primary">
          {t("recentTransactions")}
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-1 relative bg-background">
        <ScrollArea className="h-[600px] w-full rounded-md ">
          <div className="space-y-2">
            {isTransactionsLoading || transactions === undefined
              ? Array.from(Array(10)).map((_, idx) => {
                  return (
                    <TxCard
                      key={`tx-loader-${idx}`}
                      isLoading={true}
                      transaction={undefined}
                    />
                  );
                })
              : transactions.map((tx) => {
                  return (
                    <TxCard
                      key={tx.hash}
                      transaction={tx}
                      transactionReceipt={transactionReceipts?.find(
                        (transactionReceipt) =>
                          transactionReceipt.transactionHash === tx.hash
                      )}
                      paladinTransaction={paladinTransactions?.find(
                        (paladinTransaction) =>
                          paladinTransaction.id ===
                          transactionReceipts?.find(
                            (transactionReceipt) =>
                              transactionReceipt.transactionHash === tx.hash
                          )?.id
                      )}
                    />
                  );
                })}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
};
