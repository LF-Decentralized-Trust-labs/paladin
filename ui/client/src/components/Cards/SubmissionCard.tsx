import { Card, CardContent } from "@/components/ui/card";
import { IPaladinTransaction } from "@/interfaces/transactions";
import { makeHashText } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { TimeText } from "../TimeText";
import { CardDetail } from "./CardDetail";

type Props = {
  paladinTransaction: IPaladinTransaction | undefined;
  isLoading?: boolean;
};

export function SubmissionCard({ paladinTransaction, isLoading }: Props) {
  const { t } = useTranslation();

  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex justify-between items-center h-14">
          <CardDetail
            isLoading={isLoading}
            title={t("type")}
            value={t(paladinTransaction?.type ?? "")}
          />

          <CardDetail
            title={t("id")}
            value={makeHashText(paladinTransaction?.id ?? "")}
            isLoading={isLoading}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("from")}
            value={paladinTransaction?.from ?? ""}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("to")}
            value={makeHashText(paladinTransaction?.to ?? "")}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("domain")}
            value={paladinTransaction?.domain ?? "--"}
          />
          <TimeText time={paladinTransaction?.created ?? ""} />
        </div>
      </CardContent>
    </Card>
  );
}
