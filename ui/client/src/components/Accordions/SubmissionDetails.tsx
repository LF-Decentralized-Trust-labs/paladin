import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { IPaladinTransaction } from "@/interfaces/transactions";
import { useTranslation } from "react-i18next";
import { ScrollArea } from "../ui/scroll-area";

type Props = {
  paladinTransaction: IPaladinTransaction;
  mode: "details" | "properties";
};

export function SubmissionDetails({ paladinTransaction, mode }: Props) {
  const { t } = useTranslation();

  const formatProperty = (value: string) => {
    try {
      const parsed = JSON.stringify(value, null, 2);
      return parsed;
    } catch {
      return value;
    }
  };

  if (
    mode === "properties" &&
    Object.keys(paladinTransaction.data).length === 0
  ) {
    return <></>;
  }

  return (
    <div className="border p-4 pt-0 rounded">
      {mode === "properties" && (
        <Accordion type="single" collapsible>
          {Object.keys(paladinTransaction.data)
            .filter((property) => property !== "$owner")
            .map((property) => {
              return (
                <AccordionItem key={property} value={property}>
                  <AccordionTrigger>
                    {t("property")}: {property}
                  </AccordionTrigger>
                  <AccordionContent className="whitespace-pre-wrap break-words text-xs">
                    {formatProperty(paladinTransaction.data[property])}
                  </AccordionContent>
                </AccordionItem>
              );
            })}
        </Accordion>
      )}
      {mode === "details" && (
        <Accordion type="single" collapsible defaultValue={"fullTxDetails"}>
          <AccordionItem value={"fullTxDetails"}>
            <AccordionTrigger>{t("fullTransactionDetails")}</AccordionTrigger>
            <AccordionContent className="whitespace-pre-wrap break-words text-[10px] leading-tight relative -mr-4">
              <ScrollArea className=" h-[300px] w-full pr-4">
                {JSON.stringify(paladinTransaction, null, 2)}
              </ScrollArea>
            </AccordionContent>
          </AccordionItem>
        </Accordion>
      )}
    </div>
  );
}
