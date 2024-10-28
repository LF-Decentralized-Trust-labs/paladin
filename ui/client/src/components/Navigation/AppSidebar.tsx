import { NavMain } from "@/components/Navigation/NavMain";
import {
  Sidebar,
  SIDEBAR_KEYBOARD_SHORTCUT,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
} from "@/components/ui/sidebar";
import { Command } from "lucide-react";
import * as React from "react";
import { NavQueries } from "./NavQueries";
import { SidebarPaladinHeader } from "./SidebarPaladinHeader";

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  return (
    <Sidebar collapsible="icon" {...props}>
      <SidebarHeader>
        <SidebarPaladinHeader />
      </SidebarHeader>
      <SidebarContent>
        <NavMain />
        <NavQueries />
      </SidebarContent>
      <SidebarFooter>
        <span className="flex items-center gap-1 bg-transparent text-primary border border-primary w-8 h-8 justify-center p-1 rounded-md">
          <Command className="w-4 h-4" />
          <p className="text-xs">{SIDEBAR_KEYBOARD_SHORTCUT.toUpperCase()}</p>
        </span>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
