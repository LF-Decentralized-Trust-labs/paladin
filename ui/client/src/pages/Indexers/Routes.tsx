import { RouteObject } from "react-router-dom";
import { AppLinks } from "@/navigation/AppLinks";
import { Indexers } from "./views/Indexers";

export const IndexersRoute: RouteObject = {
  path: AppLinks.Indexers,
  element: <Indexers />,
  index: true,
};
