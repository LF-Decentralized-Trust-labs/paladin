// Copyright © 2025 Kaleido, Inc.
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

import { Box, Button, Chip } from "@mui/material";
import { Dispatch, SetStateAction, useState } from "react";
import { AddFilterDialog } from "../dialogs/AddFilter";
import { useTranslation } from "react-i18next";
import { IFilter, IFilterField } from "../interfaces";

type Props = {
  filterFields: IFilterField[]
  filters: IFilter[]
  setFilters: Dispatch<SetStateAction<IFilter[]>>
}

export const Filters: React.FC<Props> = ({
  filterFields,
  filters,
  setFilters
}) => {

  const [addFilterDialogOpen, setAddFilterDialogOpen] = useState(false);
  const { t } = useTranslation();

  const getOperatorLabel = (operator: string) => {
    switch (operator) {
      case 'equal': return '= ';
      case 'neq': return '!= ';
      case 'greaterThan': return '> ';
      case 'greaterThanOrEqual': return '>= ';
      case 'lessThan': return '< ';
      case 'lessThanOrEqual': return '<= ';
      case 'contains': return '= @';
      case 'startsWith': return '= ^';
      case 'endsWith': return '= $';
      case 'doesNotContain': return '= !@';
      case 'doesNotStartWith': return '= !^';
      case 'doesNotEndWith': return '= !$';
    }

  };

  const getFilterId = (filter: IFilter) => `${filter.field.name}-${filter.operator}-${filter.value}${filter.caseSensitive}`;

  const generateFilterLabel = (filter: IFilter) => {
    return `${filter.field.label} ${getOperatorLabel(filter.operator)}${filter.value}`
  };

  return (
    <>
      <Box sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'right',
        gap: '10px',
        flexWrap: 'wrap'
      }}>

        {filters.map(filter =>
          <Chip
            key={getFilterId(filter)}
            label={generateFilterLabel(filter)}
            
            onDelete={() => {
              const id = getFilterId(filter);
              setFilters(filters.filter(currentFilter => getFilterId(currentFilter) !== id));
            }}
          />
        )}

        {filters.length > 0 &&
          <Button
            size="small"
            variant="outlined"
            sx={{ borderRadius: '20px', minWidth: '100px'  }}
            onClick={() => setFilters([])}
          >
            {t('clearFilters')}
          </Button>
        }

        <Button
          size="small"
          variant="outlined"
          color="secondary"
          sx={{ borderRadius: '20px', minWidth: '100px' }}
          onClick={() => setAddFilterDialogOpen(true)}
        >
          {t('addFilter')}
        </Button>

      </Box>

      <AddFilterDialog
        filterFields={filterFields}
        addFilter={filter => setFilters([...filters, filter])}
        dialogOpen={addFilterDialogOpen}
        setDialogOpen={setAddFilterDialogOpen}
      />

    </>
  );

}