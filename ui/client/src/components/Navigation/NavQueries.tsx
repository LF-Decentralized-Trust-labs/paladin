import {
  Book,
  Box,
  Container,
  KeyRound,
  Orbit,
  type LucideIcon,
} from "lucide-react";

import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { AppLinks } from "@/navigation/AppLinks";
import { useTranslation } from "react-i18next";

export function NavQueries() {
  const { t } = useTranslation();

  const items: {
    name: string;
    url: string;
    icon: LucideIcon;
  }[] = [
    {
      name: `${t("blockIndexer")}:${t("bidx")}`,
      url: AppLinks.QueryBuilder,
      icon: Box,
    },
    {
      name: `${t("keyManager")}:${t("keymgr")}`,
      url: AppLinks.QueryBuilder,
      icon: KeyRound,
    },
    {
      name: `${t("stateStore")}:${t("pstate")}`,
      url: AppLinks.QueryBuilder,
      icon: Orbit,
    },
    {
      name: `${t("registryManager")}:${t("reg")}`,
      url: AppLinks.QueryBuilder,
      icon: Book,
    },
    {
      name: `${t("transportManager")}:${t("transport")}`,
      url: AppLinks.QueryBuilder,
      icon: Container,
    },
  ];

  return (
    <SidebarGroup className="group-data-[collapsible=icon]:hidden">
      <SidebarGroupLabel>{t("rpcQueries")}</SidebarGroupLabel>
      <SidebarMenu>
        {items.map((item) => (
          <SidebarMenuItem key={item.name}>
            <SidebarMenuButton asChild>
              <a href={item.url}>
                <item.icon />
                <span>{item.name}</span>
              </a>
            </SidebarMenuButton>
          </SidebarMenuItem>
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}
