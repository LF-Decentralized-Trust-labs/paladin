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

import { Alert, Box, Breadcrumbs, Button, Fade, Grid2, IconButton, Link, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, ToggleButton, ToggleButtonGroup, Tooltip, Typography, useTheme } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { fetchKeys } from "../queries/keys";
import { Hash } from "../components/Hash";
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import { IFilter, IKeyEntry, IVerifier } from "../interfaces";
import { useSearchParams } from "react-router-dom";
import { Captions, Signature } from "lucide-react";
import { constants } from "../components/config";
import SearchIcon from '@mui/icons-material/Search';
import { ReverseKeyLookupDialog } from "../dialogs/ReverseKeyLookup";
import RemoveIcon from '@mui/icons-material/Remove';
import { VerifiersDialog } from "../dialogs/Verifiers";
import { useTranslation } from "react-i18next";
import { Filters } from "../components/Filters";
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import ViewListIcon from '@mui/icons-material/ViewList';
import { getAltModeScrollBarStyle } from "../themes/default";

export const Keys: React.FC = () => {

  const getDefaultRowsPerPage = () => {
    const valueFromStorage = window.localStorage.getItem(constants.KEYS_ROWS_PER_PAGE);
    if (valueFromStorage !== null) {
      return Number(valueFromStorage);
    }
    return 10;
  };

  const getDefaultMode = () => {
    const valueFromStorage = window.localStorage.getItem(constants.KEYS_MODE);
    if (valueFromStorage === 'explorer' || valueFromStorage === 'list') {
      return valueFromStorage;
    }
    return 'explorer';
  };

  const getDefaultSortBy = () => {
    return window.localStorage.getItem(constants.KEYS_SORT_BY_STORAGE_KEY) ?? 'index';
  };

  const getDefaultSortOrder = () => {
    return window.localStorage.getItem(constants.KEYS_SORT_ORDER_STORAGE_KEY) as 'asc' | 'desc' ?? 'asc';
  };

  const getFiltersFromStorage = () => {
    const value = window.localStorage.getItem(constants.KEYS_FILTERS_KEY);
    if (value !== null) {
      try {
        return JSON.parse(value);
      } catch (_err) { }
    }
    return [];
  };

  const [searchParams, setSearchParams] = useSearchParams();
  const [refEntries, setRefEntries] = useState<IKeyEntry[]>([]);
  const [page, setPage] = useState(0);
  const [count, setCount] = useState(-1);
  const [rowsPerPage, setRowsPerPage] = useState(getDefaultRowsPerPage());
  const [parent, setParent] = useState(searchParams.get('path') ?? '');
  const [reverseLookupDialogOpen, setReverseLookupDialogOpen] = useState(false);
  const [sortByPathFirst, setSortByPathFirst] = useState(getDefaultSortBy() === 'path');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>(getDefaultSortOrder);
  const [selectedVerifiers, setSelectedVerifiers] = useState<IVerifier[]>();
  const [verifiersDialogOpen, setVerifiersDialogOpen] = useState(false);
  const [filters, setFilters] = useState<IFilter[]>(getFiltersFromStorage());
  const [mode, setMode] = useState<'explorer' | 'list'>(getDefaultMode());
  const { t } = useTranslation();
  const theme = useTheme();

  useEffect(() => {
    setParent(searchParams.get('path') ?? '');
  }, [searchParams]);

  const { data: keys, error } = useQuery({
    queryKey: ["keys", parent, sortByPathFirst, sortOrder, refEntries, rowsPerPage, filters, mode],
    queryFn: () => fetchKeys(mode === 'explorer' ? parent : undefined, rowsPerPage, sortByPathFirst, sortOrder, filters, refEntries[refEntries.length - 1])
  });

  useEffect(() => {
    if (count !== -1 && (page !== 0 && page * rowsPerPage === count)) {
      handleChangePage(null, page - 1);
    }
  }, [count, rowsPerPage, page]);

  useEffect(() => {
    if (keys !== undefined && count === -1) {
      if (keys.length < rowsPerPage) {
        setCount(rowsPerPage * page + keys.length);
      }
    }
  }, [keys, rowsPerPage, page]);

  useEffect(() => {
    setPage(0);
    setCount(-1);
    setRefEntries([]);
  }, [parent]);

  useEffect(() => {
    let value: any = {};
    if (parent !== '') {
      value.path = parent;
    }
    setSearchParams(value);
  }, [parent, page]);

  useEffect(() => {
    window.localStorage.setItem(constants.KEYS_FILTERS_KEY, JSON.stringify(filters));
    setCount(-1);
  }, [filters]);

  useEffect(() => {
    if (mode === 'list') {
      setParent('');
    }
    setPage(0);
    setCount(-1);
    setRefEntries([]);
    window.localStorage.setItem(constants.KEYS_MODE, mode);
  }, [mode]);

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  const handleSortChange = (column: string) => {
    if ((column === 'path' && sortByPathFirst) || (column === 'index' && !sortByPathFirst)) {
      const order = sortOrder === 'asc' ? 'desc' : 'asc';
      setSortOrder(order);
      window.localStorage.setItem(constants.KEYS_SORT_ORDER_STORAGE_KEY, order);
    } else {
      window.localStorage.setItem(constants.KEYS_SORT_BY_STORAGE_KEY, column);
      if (sortOrder !== 'asc') {
        window.localStorage.setItem(constants.KEYS_SORT_ORDER_STORAGE_KEY, 'asc');
        setSortOrder('asc');
      }
      setSortByPathFirst(column === 'path');
    }
    setPage(0);
    setRefEntries([]);
  };

  let breadcrumbContent: JSX.Element[] = [];
  if (parent !== '') {
    const segments = parent.split('.');
    let segmentStack: string[] = [];
    for (const segment of segments) {
      segmentStack.push(segment);
      const target = segmentStack.join('.');
      breadcrumbContent.push(
        <Link underline="none"
          key={segment}
          href=""
          onClick={event => {
            event.preventDefault();
            setParent(target);
          }}>
          {segment === '' ? t('root') : segment}
        </Link>
      )
    }
  }

  const getEthAddress = (key: IKeyEntry) => {
    const entry = key.verifiers?.find(entry => entry.type === 'eth_address');
    if (entry !== undefined) {
      return <Hash
        Icon={<Captions size="18px" />}
        title={entry.algorithm}
        hash={entry.verifier}
        hideTitle />
    }
    return <RemoveIcon color="disabled" />;
  };

  const getOtherVerifiers = (key: IKeyEntry) => {
    if (key.verifiers !== null) {
      const entries = key.verifiers.filter(entry => entry.type !== 'eth_address');
      if (entries.length === 1) {
        return <Hash
          Icon={<Signature size="18px" />}
          title={entries[0].algorithm}
          hash={entries[0].verifier}
          hideTitle />
      } else if (entries.length > 1) {
        return (
          <Button
            variant="contained"
            disableElevation
            color="primary"
            size="small"
            sx={{ minWidth: '110px', paddingTop: 0, paddingBottom: 0, fontWeight: '400', whiteSpace: 'nowrap' }}
            onClick={() => { setSelectedVerifiers(entries); setVerifiersDialogOpen(true) }}
          >
            {t('manyN', { n: entries.length })}
          </Button>
        );
      }
    }
    return <RemoveIcon color="disabled" />;
  };

  const removeParentFromPath = (path: string) => {
    let index = parent.length;
    if (index > 0) {
      index++;
    }
    return path.substring(index);
  }

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefEntries([]);
    } else if (newPage > page) {
      if(keys !== undefined) {
        refEntries.push(keys[keys.length - 1]);
      }
    } else {
      refEntries.pop();
    }
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    const value = parseInt(event.target.value, 10);
    setRowsPerPage(value);
    window.localStorage.setItem(constants.KEYS_ROWS_PER_PAGE, value.toString());
    setRefEntries([]);
    setPage(0);
  };

  const headerDivider = <Box sx={{
    height: '30px',
    width: '1px',
    border: theme => `solid 1px ${theme.palette.divider}`,
    position: 'absolute',
    top: '14px',
    left: '2px'
  }} />;

  return (
    <>
      <Fade timeout={300} in={true}>
        <Box
          sx={{
            padding: "20px",
            maxWidth: "1300px",
            marginLeft: "auto",
            marginRight: "auto",
          }}
        >
          <Grid2 container alignItems="center" spacing={2}>
            <Grid2 sx={{ display: { xs: 'none', sm: 'none', md: 'block' } }} size={{ md: 4 }} />
            <Grid2 size={{ xs: 12, md: 4 }}>
              <Typography align="center" variant="h5">
                {t("localKeys")}
              </Typography>
            </Grid2>
            <Grid2 size={{ xs: 12, md: 4 }} container justifyContent="right">
              <Grid2>
                <Button
                  size="large"
                  variant="outlined"
                  startIcon={<SearchIcon />}
                  sx={{ borderRadius: '20px' }}
                  onClick={() => setReverseLookupDialogOpen(true)}
                >
                  {t('reverseLookup')}
                </Button>
              </Grid2>
            </Grid2>
          </Grid2>
          <Box sx={{ height: '10px' }} />
          <Filters
            filterFields={[
              {
                label: t('path'),
                name: 'path',
                type: 'string'
              },
              {
                label: t('index'),
                name: 'index',
                type: 'number'
              },
              {
                label: t('wallet'),
                name: 'wallet',
                type: 'string'
              },
              {
                label: t('handle'),
                name: 'keyHandle',
                type: 'string'
              },
              {
                label: t('isFolder'),
                name: 'hasChildren',
                type: 'boolean'
              },
              {
                label: t('isKey'),
                name: 'isKey',
                type: 'boolean'
              }
            ]}
            filters={filters}
            setFilters={setFilters}
          />
          <Box sx={{ display: 'flex', marginBottom: '15px', alignItems: 'center' }}>
            <ToggleButtonGroup exclusive onChange={(_event, value) => setMode(value)} value={mode}>
              <Tooltip arrow title={t('listView')}>
                <ToggleButton color="primary" value="list">
                  <ViewListIcon fontSize="small" />
                </ToggleButton>
              </Tooltip>
              <Tooltip arrow title={t('explorerView')}>
                <ToggleButton color="primary" value="explorer">
                  <AccountTreeIcon fontSize="small" />
                </ToggleButton>
              </Tooltip>
            </ToggleButtonGroup>
            {mode === 'explorer' &&
              <Breadcrumbs
                separator={<NavigateNextIcon fontSize="small" />}
                sx={{ marginLeft: '10px' }}>
                <Link underline="none"
                  href=""
                  onClick={event => { event.preventDefault(); setParent('') }}>
                  {t('root')}
                </Link>
                {breadcrumbContent}
              </Breadcrumbs>}
          </Box>
          <TableContainer component={Paper} sx={{ height: 'calc(100vh - 320px)', ...getAltModeScrollBarStyle(theme.palette.mode) }}>
            <Table stickyHeader>
              <TableHead>
                <TableRow>
                  {mode === 'explorer' &&
                    <TableCell width={1} sx={{ minWidth: '70px', backgroundColor: theme => theme.palette.background.paper }} />
                  }
                  <TableCell sx={{ backgroundColor: theme => theme.palette.background.paper }}>
                    <TableSortLabel
                      active={sortByPathFirst}
                      direction={sortOrder}
                      onClick={() => handleSortChange('path')}
                    >
                      {t(mode === 'explorer' ? 'pathSegment' : 'path')}
                    </TableSortLabel>
                    {mode === 'explorer' && headerDivider}
                  </TableCell>
                  <TableCell width={1} sx={{ backgroundColor: theme => theme.palette.background.paper }}>
                    <TableSortLabel
                      active={!sortByPathFirst}
                      direction={sortOrder}
                      onClick={() => handleSortChange('index')}
                    >
                      {t('index')}
                    </TableSortLabel>
                    {headerDivider}
                  </TableCell>
                  <TableCell sx={{ minWidth: '160px', backgroundColor: theme => theme.palette.background.paper }} width={1} >{t('address')}{headerDivider}</TableCell>
                  <TableCell sx={{ minWidth: '160px', backgroundColor: theme => theme.palette.background.paper, whiteSpace: 'nowrap' }} width={1} >{t('otherVerifiers')}{headerDivider}</TableCell>
                  <TableCell sx={{ minWidth: '160px', backgroundColor: theme => theme.palette.background.paper }} width={1}>{t('wallet')}{headerDivider}</TableCell>
                  <TableCell sx={{ minWidth: '160px', backgroundColor: theme => theme.palette.background.paper }} width={1} >{t('handle')}{headerDivider}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {keys?.map(key =>
                  <TableRow sx={{ height: '70px' }} key={`${key.path}${key.index}`}>
                    {mode === 'explorer' &&
                      <TableCell>{key.hasChildren &&
                        <Tooltip arrow title={t('openFolder')}>
                          <IconButton onClick={() => { setParent(key.path) }}>
                            <FolderOpenIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      }</TableCell>}
                    <TableCell sx={{ wordBreak: 'break-all' }}>{mode === 'explorer' ? removeParentFromPath(key.path) : key.path}</TableCell>
                    <TableCell>{key.index}</TableCell>
                    <TableCell>
                      {getEthAddress(key)}
                    </TableCell>
                    <TableCell>
                      {getOtherVerifiers(key)}
                    </TableCell>
                    <TableCell sx={{ whiteSpace: 'nowrap' }}>{key.wallet.length > 0 ? key.wallet : <RemoveIcon color="disabled" />}</TableCell>
                    <TableCell>{key.keyHandle.length > 0 ?
                      <Hash hash={key.keyHandle} title={t('handle')} hideTitle secondary />
                      : <RemoveIcon color="disabled" />}</TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
          <TablePagination
            slotProps={{
              actions: {
                lastButton: {
                  disabled: true
                }
              }
            }}
            component="div"
            showFirstButton
            showLastButton
            count={count}
            page={page}
            onPageChange={handleChangePage}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={handleChangeRowsPerPage}
          />
        </Box>
      </Fade>
      <ReverseKeyLookupDialog
        dialogOpen={reverseLookupDialogOpen}
        setDialogOpen={setReverseLookupDialogOpen}
        mode={mode}
        setParent={setParent}
        setFilters={setFilters}
      />
      {selectedVerifiers &&
        <VerifiersDialog
          dialogOpen={verifiersDialogOpen}
          setDialogOpen={setVerifiersDialogOpen}
          verifiers={selectedVerifiers}
        />}

    </>
  );
}
