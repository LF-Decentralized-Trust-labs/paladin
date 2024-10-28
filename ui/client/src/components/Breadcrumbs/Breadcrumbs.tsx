import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

export interface BreadcrumbItem {
  title: string;
  href?: string;
}

interface Props {
  breadcrumbs: BreadcrumbItem[];
}

export const Breadcrumbs = ({ breadcrumbs }: Props) => {
  return (
    <Breadcrumb>
      <BreadcrumbList>
        {breadcrumbs.map((bc, index) => {
          return (
            <>
              <BreadcrumbItem className="hidden md:block" key={bc.title}>
                <BreadcrumbLink href={bc.href}>{bc.title}</BreadcrumbLink>
              </BreadcrumbItem>
              {index !== breadcrumbs.length - 1 && (
                <BreadcrumbSeparator className="hidden md:block" />
              )}
            </>
          );
        })}
      </BreadcrumbList>
    </Breadcrumb>
  );
};
