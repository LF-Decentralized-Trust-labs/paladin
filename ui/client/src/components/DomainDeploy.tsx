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
import { NotoDeployDialog } from '../dialogs/domains/noto/NotoDeploy';
import { ZetoDeployDialog } from '../dialogs/domains/zeto/ZetoDeploy';

type Props = {
  domainName: string;
};

interface DeployButton {
  action: () => void;
}

export const DomainDeploy: React.FC<Props> = ({ domainName }) => {
  const { t } = useTranslation();
  const [button, setButton] = useState<DeployButton>();
  const [notoDeployModalOpen, setNotoDeployModalOpen] = useState(false);
  const [zetoDeployModalOpen, setZetoDeployModalOpen] = useState(false);

  useEffect(() => {
    // TODO: should key off of the domain "type" instead of expecting a specific name
    switch (domainName) {
      case 'noto': {
        setButton({
          action: () => setNotoDeployModalOpen(true),
        });
        break;
      }
      case 'zeto': {
        setButton({
          action: () => setZetoDeployModalOpen(true),
        });
        break;
      }
    }
  }, [domainName]);

  return (
    <>
      <Grid2>
        {button && (
          <Button
            size="large"
            variant="outlined"
            sx={{ borderRadius: '20px' }}
            onClick={button.action}
          >
            {t('deployNew')}
          </Button>
        )}
      </Grid2>

      <NotoDeployDialog
        dialogOpen={notoDeployModalOpen}
        setDialogOpen={setNotoDeployModalOpen}
        domain={domainName}
      />

      <ZetoDeployDialog
        dialogOpen={zetoDeployModalOpen}
        setDialogOpen={setZetoDeployModalOpen}
        domain={domainName}
      />
    </>
  );
};
