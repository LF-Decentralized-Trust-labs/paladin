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

import { Button, Grid2 } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NotoMintDialog } from '../dialogs/domains/noto/NotoMint';
import { NotoTransferDialog } from '../dialogs/domains/noto/NotoTransfer';
import { ZetoMintDialog } from '../dialogs/domains/zeto/ZetoMint';
import { ZetoTransferDialog } from '../dialogs/domains/zeto/ZetoTransfer';

type Props = {
  domainName: string;
  contractAddress: string;
};

interface DomainButton {
  name: string;
  action: () => void;
}

export const DomainButtons: React.FC<Props> = ({
  domainName,
  contractAddress,
}) => {
  const { t } = useTranslation();
  const [buttons, setButtons] = useState<DomainButton[]>([]);
  const [notoMintDialogOpen, setNotoMintDialogOpen] = useState(false);
  const [notoTransferDialogOpen, setNotoTransferDialogOpen] = useState(false);
  const [zetoMintDialogOpen, setZetoMintDialogOpen] = useState(false);
  const [zetoTransferDialogOpen, setZetoTransferDialogOpen] = useState(false);

  useEffect(() => {
    const tmpButtons: DomainButton[] = [];

    // TODO: should key off of the domain "type" instead of expecting a specific name
    switch (domainName) {
      case 'noto': {
        tmpButtons.push({
          name: 'mint',
          action: () => setNotoMintDialogOpen(true),
        });
        tmpButtons.push({
          name: 'transfer',
          action: () => setNotoTransferDialogOpen(true),
        });
        break;
      }
      case 'zeto': {
        tmpButtons.push({
          name: 'mint',
          action: () => setZetoMintDialogOpen(true),
        });
        tmpButtons.push({
          name: 'transfer',
          action: () => setZetoTransferDialogOpen(true),
        });
        break;
      }
    }

    setButtons(tmpButtons);
  }, [domainName]);

  return (
    <>
      <Grid2>
        {buttons.map((button) => (
          <Button
            key={button.name}
            sx={{ fontWeight: '400' }}
            size="small"
            onClick={button.action}
          >
            {t(button.name)}
          </Button>
        ))}
        {buttons.length === 0 && t('noActions')}
      </Grid2>

      <NotoMintDialog
        dialogOpen={notoMintDialogOpen}
        setDialogOpen={setNotoMintDialogOpen}
        contractAddress={contractAddress}
      />

      <NotoTransferDialog
        dialogOpen={notoTransferDialogOpen}
        setDialogOpen={setNotoTransferDialogOpen}
        contractAddress={contractAddress}
      />

      <ZetoMintDialog
        dialogOpen={zetoMintDialogOpen}
        setDialogOpen={setZetoMintDialogOpen}
        contractAddress={contractAddress}
      />

      <ZetoTransferDialog
        dialogOpen={zetoTransferDialogOpen}
        setDialogOpen={setZetoTransferDialogOpen}
        contractAddress={contractAddress}
      />
    </>
  );
};
