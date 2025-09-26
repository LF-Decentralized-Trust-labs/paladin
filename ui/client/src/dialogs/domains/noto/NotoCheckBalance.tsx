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

import { Dispatch, SetStateAction } from 'react';
import { CheckBalanceDialog } from '../../CheckBalance';

type Props = {
  contractAddress: string;
  dialogOpen: boolean;
  setDialogOpen: Dispatch<SetStateAction<boolean>>;
};

export const NotoCheckBalanceDialog: React.FC<Props> = ({
  contractAddress,
  dialogOpen,
  setDialogOpen,
}) => {
  return (
    <CheckBalanceDialog
      domain="noto"
      contractAddress={contractAddress}
      dialogOpen={dialogOpen}
      setDialogOpen={setDialogOpen}
    />
  );
}; 