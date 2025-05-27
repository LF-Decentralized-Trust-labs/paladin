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

import { IFilter } from './interfaces';

export const formatJSONWhenApplicable = (value: any) => {
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value, null, 2);
    } catch (err) {}
  }
  return String(value);
};

export const translateFilters = (filters: IFilter[]) => {
  let result: any = {};

  for (const filter of filters) {
    let entry: any = {
      field: filter.field.name,
      value: filter.value,
    };

    if (filter.caseSensitive === false) {
      entry.caseInsensitive = true;
    }

    let operator = filter.operator;

    switch (operator) {
      case 'contains':
        operator = 'like';
        entry.value = `%${entry.value}%`;
        break;
      case 'startsWith':
        operator = 'like';
        entry.value = `${entry.value}%`;
        break;
      case 'endsWith':
        operator = 'like';
        entry.value = `%${entry.value}`;
        break;
      case 'doesNotContain':
        operator = 'like';
        entry.not = true;
        entry.value = `%${entry.value}%`;
        break;
      case 'doesNotStartWith':
        operator = 'like';
        entry.not = true;
        entry.value = `${entry.value}%`;
        break;
      case 'doesNotEndWith':
        operator = 'like';
        entry.not = true;
        entry.value = `%${entry.value}`;
        break;
    }

    let group = result[operator] ?? [];
    group.push(entry);
    result[operator] = group;
  }

  return result;
};

export const isValidUUID = (uuid: string) =>
  /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$/.test(
    uuid
  );

export const encodeHex = (str: string) =>
  '0x' +
  [...new TextEncoder().encode(str)]
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');

// Infer the base path from the current URL
// Assume that the base path is the part of the URL up to the "/ui" segment
export const getBasePath = () => {
  const pathname = window.location.pathname;
  const pathSegments = pathname.split('/');
  for (let i = 0; i < pathSegments.length; i++) {
    if (pathSegments[i] === 'ui') {
      // pathSegments[0] is the empty string, so we need to avoid ending up with //something
      return ('/' + pathSegments.slice(0, i).join('/')).replace(/^\/\/+/, '/');
    }
  }
  return '/';
};
