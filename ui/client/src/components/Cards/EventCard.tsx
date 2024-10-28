import { Card, CardContent } from "@/components/ui/card";
import { IEvent } from "@/interfaces/events";
import { useTranslation } from "react-i18next";
import { HashChip } from "../Chips/HashChip";
import { CardDetail } from "./CardDetail";

type Props = {
  event: IEvent | undefined;
  isLoading?: boolean;
};

export function EventCard({ event, isLoading }: Props) {
  const { t } = useTranslation();

  return (
    <Card className="min-h-[140px]">
      <CardContent className="py-2">
        {/* Top row */}
        <div className="flex justify-between items-center h-20">
          <CardDetail
            isLoading={isLoading}
            title={t("block")}
            value={event?.blockNumber ?? ""}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("transactionIndex")}
            value={event?.transactionIndex ?? ""}
          />
          <CardDetail
            title={t("logIndex")}
            value={event?.logIndex ?? ""}
            isLoading={isLoading}
          />
        </div>
        {/* Bottom row */}
        <div className="flex justify-between items-center min-h-14 border-t border-border space-x-2 ">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-1 pt-2">
            <HashChip
              hash={event?.transactionHash ?? ""}
              isTransaction
              preText={t("txHash")}
            />
            <HashChip
              hash={event?.signature ?? ""}
              isFunction
              preText={t("signature")}
            />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
