import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { makeHashText } from "@/lib/utils";
import {
  AudioLines,
  Box,
  Braces,
  Captions,
  ClipboardCheck,
  Clock,
  FileText,
  FunctionSquare,
  Hash,
  ListEnd,
} from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Badge } from "../ui/badge";

interface Props {
  hash: string;
  truncate?: boolean;
  isBlock?: boolean;
  isHash?: boolean;
  isTransaction?: boolean;
  isAddress?: boolean;
  isContract?: boolean;
  isFunction?: boolean;
  isTime?: boolean;
  isJson?: boolean;
  isEvent?: { txHash: string; index: number };
  hasLink?: boolean;
  hasIcon?: boolean;
  preText?: string;
  linkOverride?: string;
  labelOverride?: string;
}

export const HashChip = ({
  hash,
  isBlock = false,
  isTransaction = false,
  isHash = false,
  isAddress = false,
  isContract = false,
  isFunction = false,
  isEvent = undefined,
  isJson = false,
  isTime,
  hasLink = true,
  hasIcon = true,
  preText,
  linkOverride,
  labelOverride,
  truncate = true,
}: Props) => {
  const navigate = useNavigate();
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
          <Badge
            variant={"outline"}
            className="flex space-x-1 font-mono cursor-pointer"
            onClick={(e) => {
              if (!hasLink) {
                return;
              }
              if (linkOverride) {
                navigate(linkOverride);
                return;
              }
              e.stopPropagation();
              if (!hasCopied) {
                navigator.clipboard.writeText(hash);
                setHasCopied(true);
              }
            }}
          >
            {/* Icons */}
            {isBlock && hasIcon && !hasCopied && (
              <span>
                <Box className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {isTransaction && hasIcon && !hasCopied && (
              <span>
                <ListEnd className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {isHash && hasIcon && !hasCopied && (
              <span>
                <Hash className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {isEvent && hasIcon && !hasCopied && (
              <span>
                <AudioLines className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {isAddress && hasIcon && !hasCopied && (
              <span>
                <Captions className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {isContract && hasIcon && !hasCopied && (
              <span>
                <FileText className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {isFunction && hasIcon && !hasCopied && (
              <span>
                <FunctionSquare className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {isJson && hasIcon && !hasCopied && (
              <span>
                <Braces className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {isTime && hasIcon && !hasCopied && (
              <span>
                <Clock className="h-[0.8rem] w-[0.8rem] text-primary " />
              </span>
            )}
            {hasCopied && (
              <div>
                <ClipboardCheck className="h-[0.8rem] w-[0.8rem] text-primary" />
              </div>
            )}
            {preText && (
              <p className="text-muted-foreground text-xs font-normal">
                {preText}&nbsp;{"|"}&nbsp;
              </p>
            )}
            {/* Hash text */}
            <p className="text-muted-foreground font-semibold truncate max-w-[200px]">
              {labelOverride ?? makeHashText(hash, truncate)}
            </p>
          </Badge>
        </TooltipTrigger>
        {/* Tooltip */}
        <TooltipContent>
          {isJson ? (
            <pre className="font-mono break-all max-h-[250px] overflow-y-scroll max-w-[500px] scrollbar-thin scrollbar-thumb-muted-foreground scrollbar-track-muted">
              {hash}
            </pre>
          ) : (
            <p className="font-mono break-all max-w-[500px]">{hash}</p>
          )}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
};
