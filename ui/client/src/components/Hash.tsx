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

import { Button } from "@mui/material";
import { useState } from "react";
import { HashDialog } from "../dialogs/Hash";

const MAX_LENGTH_WITHOUT_COLLAPSE = 16;

type Props = {
  Icon?: JSX.Element
  title: string
  hideTitle?: boolean
  hash: string
  secondary?: boolean
}

export const Hash: React.FC<Props> = ({ Icon, title, hideTitle, hash, secondary }) => {

  const [hashDialogOpen, setHashDialogOpen] = useState(false);

  const getHash = (value: string) => {
    if (value.length < MAX_LENGTH_WITHOUT_COLLAPSE) {
      return hash;
    }
    return `${value.substring(0, 6)}...${value.substring(value.length - 4)}`
  };

  const content = hideTitle ? getHash(hash) : `${title} | ${getHash(hash)}`

  return (
    <>
      <Button
        startIcon={Icon}
        disableElevation
        onClick={() => setHashDialogOpen(true)}
        fullWidth
        variant="contained"
        color={ secondary? 'secondary' : 'primary' }
        sx={{ paddingTop: 0, paddingBottom: 0, fontWeight: '400', whiteSpace: 'nowrap' }}
        size="small">
        {content}
      </Button>
      <HashDialog dialogOpen={hashDialogOpen} setDialogOpen={setHashDialogOpen} title={title} hash={hash} />
    </>
  );

};