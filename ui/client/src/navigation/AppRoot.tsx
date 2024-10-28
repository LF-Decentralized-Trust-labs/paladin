import { AppSidebar } from "@/components/Navigation/AppSidebar";
import { SidebarProvider } from "@/components/ui/sidebar";
import { useEffect } from "react";
import {
  Outlet,
  ScrollRestoration,
  useLocation,
  useNavigate,
} from "react-router-dom";
import { AppLinks } from "./AppLinks";

const AppRoot = () => {
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => {
    if (location.pathname === "/") {
      navigate(AppLinks.Indexers);
    }
  }, [location, navigate]);

  return (
    <div>
      <ScrollRestoration />
      <SidebarProvider>
        <AppSidebar />
        <Outlet />
      </SidebarProvider>
    </div>
  );
};
export default AppRoot;
