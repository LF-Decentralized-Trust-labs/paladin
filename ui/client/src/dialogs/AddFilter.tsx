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

import {
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid2,
  MenuItem,
  TextField
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IFilter, IFilterField } from '../interfaces';
import { isValidUUID } from '../utils';

type Props = {
  filterFields: IFilterField[]
  addFilter: (filter: IFilter) => void
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const AddFilterDialog: React.FC<Props> = ({
  filterFields,
  addFilter,
  dialogOpen,
  setDialogOpen
}) => {

  const [selectedFilterField, setSelectedFilterField] = useState<IFilterField>();
  const [operators, setOperators] = useState<JSX.Element[]>([]);
  const [selectedOperator, setSelectedOperator] = useState<string>();
  const [isCaseSensitive, setIsCaseSensitive] = useState(false);
  const [values, setValues] = useState<JSX.Element[]>([]);
  const [value, setValue] = useState('');
  const { t } = useTranslation();

  useEffect(() => {
    if (!dialogOpen) {
      setTimeout(() => {
        setSelectedFilterField(undefined);
        setSelectedOperator(undefined);
        setValue('');
        setIsCaseSensitive(false);
      }, 200);
    }
  }, [dialogOpen]);

  useEffect(() => {
    if (selectedFilterField !== undefined) {
      let availableOperators: JSX.Element[] = [
        <MenuItem key="equal" value="equal">{t('equal')}</MenuItem>
      ];
      let availableValues: JSX.Element[] = [];

      if (selectedFilterField.type === 'boolean') {
        availableValues = [
          <MenuItem key="true" value="true">{t('true')}</MenuItem>,
          <MenuItem key="false" value="false">{t('false')}</MenuItem>
        ];
      } else {
        availableOperators = [
          ...availableOperators,
          <MenuItem key="neq" value="neq">{t('notEqual')}</MenuItem>,
          <MenuItem key="greaterThan" value="greaterThan">{t('greaterThan')}</MenuItem>,
          <MenuItem key="greaterThanOrEqual" value="greaterThanOrEqual">{t('greaterThanOrEqual')}</MenuItem>,
          <MenuItem key="lessThan" value="lessThan">{t('lessThan')}</MenuItem>,
          <MenuItem key="lessThanOrEqual" value="lessThanOrEqual">{t('lessThanOrEqual')}</MenuItem>]
        if (selectedFilterField?.type === 'string') {
          availableOperators = [
            ...availableOperators,
            <MenuItem key="contains" value="contains">{t('contains')}</MenuItem>,
            <MenuItem key="startsWith" value="startsWith">{t('startsWith')}</MenuItem>,
            <MenuItem key="endsWith" value="endsWith">{t('endsWith')}</MenuItem>,
            <MenuItem key="doesNotContain" value="doesNotContain">{t('doesNotContain')}</MenuItem>,
            <MenuItem key="doesNotStartWith" value="doesNotStartWith">{t('doesNotStartWith')}</MenuItem>,
            <MenuItem key="doesNotEndWith" value="doesNotEndWith">{t('doesNotEndWith')}</MenuItem>];
        }
      }
      if (!availableOperators.some(operator => operator.key === selectedOperator)) {
        setSelectedOperator('equal');
      }
      if (selectedFilterField.type === 'number' && isNaN(Number(value))) {
        setValue('');
      }
      if (selectedFilterField.isUUID || (selectedOperator !== undefined
        && ['greaterThan', 'greaterThanOrEqual', 'lessThan', 'lessThanOrEqual'].includes(selectedOperator))) {
        setIsCaseSensitive(true);
      }
      setValues(availableValues);
      setOperators(availableOperators);
    }
  }, [selectedFilterField, selectedOperator]);

  const handleSubmit = () => {
    if (selectedFilterField !== undefined && selectedOperator !== undefined) {
      addFilter({
        field: selectedFilterField,
        operator: selectedOperator,
        value,
        caseSensitive: selectedFilterField?.type === 'string' ? isCaseSensitive : undefined
      });
      setDialogOpen(false);
    }
  };

  let valueHelperText: string | undefined = undefined;
  if(selectedFilterField?.isUUID) {
    valueHelperText = t('mustBeAValidUUID')
  } else if(selectedFilterField?.isHexValue) {
    valueHelperText = t('mustBeAValidHex')
  }

  const canSubmit = selectedFilterField !== undefined
    && selectedOperator !== undefined
    && (selectedFilterField.type === 'boolean' || value.length > 0)
    && (selectedFilterField.isUUID !== true || isValidUUID(value))
    && (selectedFilterField.isHexValue !== true || value.startsWith('0x'))

  return (
    <Dialog
      open={dialogOpen}
      onClose={() => setDialogOpen(false)}
      fullWidth
      maxWidth="xs"
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle sx={{ textAlign: 'center' }}>
          {t('addFilter')}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '5px' }}>
            <Grid2 container spacing={2}>
              <Grid2 size={{ xs: 12 }}>
                <TextField
                  label={t('field')}
                  autoComplete="off"
                  fullWidth
                  value={selectedFilterField?.name ?? ''}
                  onChange={event => {
                    setSelectedFilterField(filterFields.find(filterField => filterField.name === event.target.value))
                  }}
                  select
                >
                  {filterFields.map(filterField =>
                    <MenuItem key={filterField.name} value={filterField.name}>{filterField.label}</MenuItem>
                  )}
                </TextField>
              </Grid2>
              <Grid2 size={{ xs: 12 }} textAlign="center">
                <TextField
                  sx={{ textAlign: 'left' }}
                  label={t('operator')}
                  autoComplete="off"
                  fullWidth
                  value={selectedOperator ?? ''}
                  onChange={event => setSelectedOperator(event.target.value)}
                  select
                  disabled={selectedFilterField === undefined}
                >
                  {operators}
                </TextField>
              </Grid2>
              <Grid2 size={{ xs: 12 }}>
                <TextField
                  type={selectedFilterField?.type === 'number' ? 'number' : 'text'}
                  label={t('value')}
                  helperText={valueHelperText}
                  autoComplete="off"
                  fullWidth
                  disabled={selectedFilterField === undefined}
                  value={value}
                  onChange={event => setValue(event.target.value)}
                  select={selectedFilterField?.type === 'boolean'}
                >
                  {values}
                </TextField>
                <Box sx={{ textAlign: 'center' }}>
                  <FormControlLabel
                    disabled={selectedFilterField === undefined || selectedFilterField.isUUID || selectedFilterField.type !== 'string'
                      || (selectedOperator !== undefined &&
                        ['greaterThan', 'greaterThanOrEqual', 'lessThan', 'lessThanOrEqual'].includes(selectedOperator))
                    }
                    control={<Checkbox checked={isCaseSensitive} onChange={event => setIsCaseSensitive(event.target.checked)} />}
                    label={t('caseSensitive')} />
                </Box>
              </Grid2>
            </Grid2>
          </Box>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', paddingBottom: '20px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit">
            {t('add')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            disableElevation
            onClick={() => setDialogOpen(false)}
          >
            {t('cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
