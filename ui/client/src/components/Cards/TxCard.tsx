import { Card, CardContent } from "@/components/ui/card";
import {
  IPaladinTransaction,
  ITransaction,
  ITransactionReceipt,
} from "@/interfaces/transactions";
import { CircleCheck, CircleX } from "lucide-react";
import { useTranslation } from "react-i18next";
import { HashChip } from "../Chips/HashChip";
import { PaladinStatus } from "../Chips/PaladinStatus";
import { CardDetail } from "./CardDetail";

type Props = {
  transaction: ITransaction | undefined;
  transactionReceipt?: ITransactionReceipt;
  paladinTransaction?: IPaladinTransaction;
  isLoading?: boolean;
};

export function TxCard({ transaction, paladinTransaction, isLoading }: Props) {
  const { t } = useTranslation();

  return (
    <Card className="min-h-[140px] relative ">
      <CardContent className="py-2">
        {/* Top row */}
        <div className="flex justify-between items-center min-h-20">
          <CardDetail
            isLoading={isLoading}
            title={t("block")}
            value={transaction?.blockNumber ?? ""}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("transactionIndex")}
            value={transaction?.transactionIndex ?? ""}
          />
          <CardDetail
            title={t("nonce")}
            value={transaction?.nonce ?? ""}
            isLoading={isLoading}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("result")}
            value={
              transaction?.result === "success" ? (
                <CircleCheck className="text-accent" />
              ) : (
                <CircleX className="text-destructive" />
              )
            }
          />
        </div>
        {/* Bottom row */}
        <div className="flex justify-between items-center min-h-14 border-t border-border space-x-2 ">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-1 pt-2">
            <HashChip
              hash={transaction?.hash ?? ""}
              isHash
              preText={t("hash")}
            />
            <HashChip
              hash={transaction?.from ?? ""}
              isAddress
              preText={t("from")}
            />
            {transaction?.contractAddress && (
              <HashChip
                hash={transaction.contractAddress}
                isContract
                preText={t("contract")}
              />
            )}
          </div>
        </div>
        {paladinTransaction !== undefined && (
          <div className="absolute left-0 bottom-0">
            <PaladinStatus />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
