import {
  SidebarMenu,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { useTranslation } from "react-i18next";
import { PaladinIcon } from "../Icons/PaladinIcon";

export function SidebarPaladinHeader() {
  const { t } = useTranslation();
  const { open } = useSidebar();

  return (
    <SidebarMenu>
      <SidebarMenuItem className="flex items-center gap-2 p-2 pl-0 pb-0">
        <div className="flex aspect-square size-8 items-center border border-sidebar-border justify-center rounded-lg text-sidebar-primary-foreground">
          <PaladinIcon className="h-6 w-6" />
        </div>
        {open && (
          <div className="grid flex-1 text-left text-sm leading-tight">
            <span className="font-normal font-mono text-2xl text-accent uppercase tracking-widest">
              {t("paladin")}
            </span>
          </div>
        )}
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
