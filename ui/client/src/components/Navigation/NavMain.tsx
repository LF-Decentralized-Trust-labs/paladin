"use client";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  SidebarGroup,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@/components/ui/sidebar";
import { AppLinks } from "@/navigation/AppLinks";
import {
  ChevronRight,
  Combine,
  Grid2x2,
  SquareChevronRight,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { useLocation } from "react-router-dom";

export function NavMain() {
  const { t } = useTranslation();
  const pathname = useLocation().pathname.toLowerCase();

  const navItems = [
    {
      title: t("indexers"),
      icon: Combine,
      isActive: pathname.startsWith(AppLinks.Indexers),
      items: [
        {
          title: t("dashboard"),
          url: AppLinks.Indexers,
        },
      ],
    },
    {
      title: t("submissions"),
      icon: SquareChevronRight,
      isActive: pathname.startsWith(AppLinks.Submissions),
      items: [
        {
          title: t("dashboard"),
          url: AppLinks.Submissions,
        },
      ],
    },
    {
      title: t("registries"),
      icon: Grid2x2,
      isActive: pathname.startsWith(AppLinks.Registries),
      items: [
        {
          title: t("dashboard"),
          url: AppLinks.Registries,
        },
      ],
    },
  ];

  return (
    <SidebarGroup>
      <SidebarMenu>
        {navItems.map((item) => (
          <Collapsible
            key={item.title}
            asChild
            defaultOpen={item.isActive}
            className="group/collapsible"
          >
            <SidebarMenuItem>
              <CollapsibleTrigger asChild>
                <SidebarMenuButton tooltip={item.title}>
                  {item.icon && <item.icon />}
                  <span>{item.title}</span>
                  <ChevronRight className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
                </SidebarMenuButton>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <SidebarMenuSub>
                  {item.items?.map((subItem) => (
                    <SidebarMenuSubItem key={subItem.title}>
                      <SidebarMenuSubButton asChild>
                        <a href={subItem.url}>
                          <span>{subItem.title}</span>
                        </a>
                      </SidebarMenuSubButton>
                    </SidebarMenuSubItem>
                  ))}
                </SidebarMenuSub>
              </CollapsibleContent>
            </SidebarMenuItem>
          </Collapsible>
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}
