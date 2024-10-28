// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { RegistryDetails } from "@/components/Accordions/RegistryDetails";
import { RegistryCard } from "@/components/Cards/RegistryCard";
import PageLayout from "@/components/Layouts/PageLayout";
import { Registry } from "@/components/Registry";
import { ScrollArea } from "@/components/ui/scroll-area";
import { IRegistryEntryWithProperties } from "@/interfaces/registry";
import { useRegQueries } from "@/queries/reg";
import { useState } from "react";
import { useTranslation } from "react-i18next";

export const Registries: React.FC = () => {
  const { t } = useTranslation();
  const [selectedRegistry, setSelectedRegistry] = useState<
    IRegistryEntryWithProperties | undefined
  >(undefined);

  const { useListRegistries } = useRegQueries();
  const { data: registries, isLoading: loadingRegistries } =
    useListRegistries();

  return (
    <PageLayout breadcrumbs={[{ title: t("registries") }]} noPadding>
      <div className="grid grid-cols-3">
        <div className="border-r col-span-1">
          <ScrollArea className=" h-[calc(100vh-64px)] ">
            <div className="flex flex-col">
              {loadingRegistries &&
                Array.from(Array(10)).map((_, idx) => {
                  return (
                    <Registry
                      key={`registry-loader-${idx}`}
                      registryName={""}
                      isLoading={loadingRegistries}
                      onClick={() => {}}
                    />
                  );
                })}
              {registries?.map((registry) => (
                <Registry
                  key={registry}
                  registryName={registry}
                  isLoading={loadingRegistries}
                  onClick={(registry) => {
                    setSelectedRegistry(registry);
                  }}
                />
              ))}
              {!loadingRegistries && registries?.length === 0 && (
                <div className="p-10 flex justify-center items-center">
                  <p className="text-muted-foreground">{t("noRegistries")}</p>
                </div>
              )}
            </div>
          </ScrollArea>
        </div>
        {selectedRegistry && (
          <div className="p-4 space-y-4 h-[calc(100vh-64px-64px)] w-full col-span-2">
            <div className="space-y-4">
              <RegistryCard
                registryEntry={selectedRegistry}
                isLoading={false}
              />
              <RegistryDetails
                mode="properties"
                registryEntry={selectedRegistry}
              />
              <RegistryDetails
                mode="details"
                registryEntry={selectedRegistry}
              />
            </div>
          </div>
        )}
      </div>
    </PageLayout>
  );
};
