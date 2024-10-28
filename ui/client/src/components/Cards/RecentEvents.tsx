import { Config } from "@/config";
import { useBidxQueries } from "@/queries/bidx";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { ScrollArea } from "../ui/scroll-area";
import { EventCard } from "./EventCard";

export const RecentEvents = () => {
  const { t } = useTranslation();

  const { useQueryIndexedEvents } = useBidxQueries();

  const { data: events, isLoading: isLoadingEvents } = useQueryIndexedEvents({
    limit: Config.EVENT_QUERY_LIMIT,
    sort: ["blockNumber DESC", "transactionIndex DESC", "logIndex DESC"],
  });

  return (
    <Card className="border rounded-md">
      <CardHeader className="py-4 bg-background">
        <CardTitle className="flex justify-between  text-xl text-primary">
          {t("recentEvents")}
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-1 relative bg-background">
        <ScrollArea className="h-[600px] w-full rounded-md">
          <div className="space-y-2">
            {isLoadingEvents || events === undefined
              ? Array.from(Array(10)).map((_, idx) => {
                  return (
                    <EventCard
                      key={`event-loader-${idx}`}
                      isLoading={true}
                      event={undefined}
                    />
                  );
                })
              : events.map((event) => {
                  return <EventCard key={event.signature} event={event} />;
                })}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
};
