import { ClipboardCheck } from "lucide-react";
import { useState } from "react";
import { Skeleton } from "../ui/skeleton";

type Props = {
  title: string;
  value: string | number | JSX.Element;
  isLoading?: boolean;
};

export function CardDetail({ title, value, isLoading }: Props) {
  const [showCheck, setShowCheck] = useState(false);
  const Loader = <Skeleton className="h-6 w-10" />;

  const handleClick = async () => {
    const textValue =
      typeof value === "string" || typeof value === "number"
        ? String(value)
        : "";

    if (textValue) {
      await navigator.clipboard.writeText(textValue);
      setShowCheck(true);
      setTimeout(() => setShowCheck(false), 2000);
    }
  };

  return (
    <div className="flex flex-col items-center h-12">
      <div
        className="text-lg leading-7 font-medium text-primary flex items-center gap-2 cursor-pointer"
        onClick={handleClick}
      >
        {isLoading ? (
          Loader
        ) : (
          <>{showCheck ? <ClipboardCheck className="text-accent" /> : value}</>
        )}
      </div>
      <div className="text-sm font-normal text-secondary-foreground mt-auto">
        {title}
      </div>
    </div>
  );
}
