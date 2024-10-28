import { RouteObject } from "react-router-dom";
import { AppLinks } from "@/navigation/AppLinks";
import { Registries } from "./views/Registries";

export const RegistriesRoute: RouteObject = {
  path: AppLinks.Registries,
  element: <Registries />,
  index: true,
};
