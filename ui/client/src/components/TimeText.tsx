import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn, getPaladinTime } from "@/lib/utils";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

interface Props {
  time: string;
  size?: "sm" | "md";
}

export const TimeText = ({ time, size = "sm" }: Props) => {
  const { t } = useTranslation();
  const [hasCopied, setHasCopied] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => {
      setHasCopied(false);
    }, 2000);
    return () => {
      clearTimeout(timer);
    };
  }, [hasCopied]);

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger>
          <span
            className={cn(
              "text-muted-foreground text-xs font-medium",
              size === "sm" ? "text-xs" : "text-sm"
            )}
            onClick={(e) => {
              e.stopPropagation();
              if (!hasCopied) {
                navigator.clipboard.writeText(time);
                setHasCopied(true);
              }
            }}
          >
            {hasCopied ? (
              <span className="text-primary font-bold">{t("copied")}</span>
            ) : (
              getPaladinTime(time)
            )}
          </span>
        </TooltipTrigger>
        <TooltipContent>{getPaladinTime(time, true)}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
};
