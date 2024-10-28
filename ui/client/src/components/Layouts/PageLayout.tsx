import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarTrigger } from "@/components/ui/sidebar";
import { BreadcrumbItem, Breadcrumbs } from "../Breadcrumbs/Breadcrumbs";
import { ThemeToggle } from "../ThemeToggle";
import { Badge } from "../ui/badge";
import { useContext } from "react";
import { ApplicationContext } from "@/contexts/ApplicationContext";
import { cn } from "@/lib/utils";

interface Props {
  children: React.ReactNode;
  breadcrumbs: BreadcrumbItem[];
  noPadding?: boolean;
}

export default function PageLayout({
  children,
  breadcrumbs,
  noPadding = false,
}: Props) {
  const { nodeName } = useContext(ApplicationContext);

  return (
    <SidebarInset>
      <header className="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-[[data-collapsible=icon]]/sidebar-wrapper:h-12 border-b border-border">
        <div className="flex items-center gap-2 px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <Breadcrumbs breadcrumbs={breadcrumbs} />
        </div>
        <div className="ml-auto px-4 flex items-center gap-2">
          <Badge variant="outline">{nodeName}</Badge>
          <ThemeToggle />
        </div>
      </header>
      <div
        className={cn(
          "flex flex-1 flex-col gap-4 pt-0",
          noPadding ? "" : "p-4"
        )}
      >
        {children}
      </div>
    </SidebarInset>
  );
}
