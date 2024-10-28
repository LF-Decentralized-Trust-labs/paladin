import { Card, CardContent } from "@/components/ui/card";
import { IRegistryEntryWithProperties } from "@/interfaces/registry";
import { makeHashText } from "@/lib/utils";
import { CircleCheck, CircleX } from "lucide-react";
import { useTranslation } from "react-i18next";
import { CardDetail } from "./CardDetail";

type Props = {
  registryEntry: IRegistryEntryWithProperties | undefined;
  isLoading?: boolean;
};

export function RegistryCard({ registryEntry, isLoading }: Props) {
  const { t } = useTranslation();

  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex justify-between items-center h-14">
          <CardDetail
            isLoading={isLoading}
            title={t("name")}
            value={t(registryEntry?.name ?? "")}
          />
          <CardDetail
            title={t("registry")}
            value={registryEntry?.registry ?? ""}
            isLoading={isLoading}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("id")}
            value={makeHashText(registryEntry?.id ?? "")}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("owner")}
            value={makeHashText(registryEntry?.properties?.$owner ?? "--")}
          />
          <CardDetail
            isLoading={isLoading}
            title={t("active")}
            value={
              registryEntry?.active ? (
                <CircleCheck className="text-accent" />
              ) : (
                <CircleX className="text-destructive" />
              )
            }
          />
        </div>
      </CardContent>
    </Card>
  );
}
