import { AppLinks } from "@/navigation/AppLinks";
import { RouteObject } from "react-router-dom";
import { QueryBuilder } from "./views/QueryBuilder";

export const QueryBuilderRoute: RouteObject = {
  path: AppLinks.QueryBuilder,
  element: <QueryBuilder />,
  index: true,
};
