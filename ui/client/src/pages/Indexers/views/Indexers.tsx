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

import { RecentEvents } from "@/components/Cards/RecentEvents";
import { RecentTransactions } from "@/components/Cards/RecentTransactions";
import PageLayout from "@/components/Layouts/PageLayout";
import { useTranslation } from "react-i18next";

export const Indexers: React.FC = () => {
  const { t } = useTranslation();

  return (
    <PageLayout breadcrumbs={[{ title: t("indexers") }]}>
      <div className="grid grid-cols-2 gap-4">
        <div>
          <RecentTransactions />
        </div>
        <div>
          <RecentEvents />
        </div>
      </div>
    </PageLayout>
  );
};
