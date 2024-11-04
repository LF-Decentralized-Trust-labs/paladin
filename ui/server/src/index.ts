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

import express from 'express';
import { createProxyMiddleware } from 'http-proxy-middleware';
import type { Request, Response } from 'express';
import path from 'path';
import pino from 'pino';

const PORT = process.env['PORT'] ?? 3555;
const PALADIN_NODE_URI =
  process.env['PALADIN_NODE_URI'] ?? 'http://127.0.0.1:31548';
const logger = pino({ transport: { target: 'pino-pretty' } });

const app = express();

const proxyMiddleware = createProxyMiddleware<Request, Response>({
  target: PALADIN_NODE_URI,
  changeOrigin: true,
});

const router = express.Router();

app.post('/', proxyMiddleware);
router.use(express.static('../client/dist'));
router.get('*', (_req, res) => {
  res.sendFile(path.resolve(__dirname + '/../../client/dist/index.html'));
});

app.use('/ui', router);

app.listen(PORT, () => {
  logger.info(`Paladin UI server running on port ${PORT}`);
});
