/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
package grpctransport

import (
	"crypto/tls"
	"fmt"
)

type GRPCConfig struct {
	ServerCertificate *tls.Certificate
	ClientCertificate *tls.Certificate
	ExternalPort      int
}

type UnprocessedGRPCConfig struct {
	ServerCertificate *string `yaml:"serverCertificate"`
	ServerKey         *string `yaml:"serverKey"`
	ClientCertificate *string `yaml:"clientCertificate"`
	ClientKey         *string `yaml:"clientKey"`
	ExternalPort      int     `yaml:"externalPort"`
}

func ProcessGRPCConfig(upc *UnprocessedGRPCConfig) (*GRPCConfig, error) {
	if upc == nil {
		return nil, fmt.Errorf("no unprocessed config provided")
	}

	config := &GRPCConfig{
		ExternalPort: upc.ExternalPort,
	}

	if upc.ServerCertificate != nil && len(*upc.ServerCertificate) != 0 && upc.ServerKey != nil && len(*upc.ServerKey) != 0 {
		cert, err := tls.X509KeyPair([]byte(*upc.ServerCertificate), []byte(*upc.ServerKey))
		if err != nil {
			return nil, err
		}

		config.ServerCertificate = &cert
	}

	if upc.ClientCertificate != nil && len(*upc.ClientCertificate) != 0 && upc.ClientKey != nil && len(*upc.ClientKey) != 0 {
		cert, err := tls.X509KeyPair([]byte(*upc.ClientCertificate), []byte(*upc.ClientKey))
		if err != nil {
			return nil, err
		}

		config.ClientCertificate = &cert
	}

	return config, nil
}
