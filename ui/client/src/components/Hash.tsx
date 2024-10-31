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

import { ButtonBase, Typography } from "@mui/material";
import { useState } from "react";
import { HashDialog } from "../dialogs/Hash";

const MAX_LENGTH_WITHOUT_COLLAPSE = 16;

type Props = {
  title: string
  hash: string
}

export const Hash: React.FC<Props> = ({ title, hash }) => {

  const [hashDialogOpen, setHashDialogOpen] = useState(false);

  if(hash.length < MAX_LENGTH_WITHOUT_COLLAPSE) {
    return (
      <Typography variant="h6" color="inherit">{hash}</Typography>
    );
  }

  return (
    <>
      <ButtonBase onClick={() => setHashDialogOpen(true)}>
        <Typography variant="h6" color="primary">{`${hash.substring(0, 5)}...${hash.substring(hash.length - 4)}`}</Typography>
      </ButtonBase>
      <HashDialog dialogOpen={hashDialogOpen} setDialogOpen={setHashDialogOpen} title={title} hash={hash} />
    </>
  );

};