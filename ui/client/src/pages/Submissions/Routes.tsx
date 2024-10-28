import { AppLinks } from "@/navigation/AppLinks";
import { RouteObject } from "react-router-dom";
import { Submissions } from "./views/Submissions";

export const SubmissionsRoute: RouteObject = {
  path: AppLinks.Submissions,
  element: <Submissions />,
  index: true,
};
