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

package tlsconf

type Config struct {
	Enabled                bool              `yaml:"enabled"`
	ClientAuth             bool              `yaml:"clientAuth,omitempty"`
	CAFile                 string            `yaml:"caFile,omitempty"`
	CA                     string            `yaml:"ca,omitempty"`
	CertFile               string            `yaml:"certFile,omitempty"`
	Cert                   string            `yaml:"cert,omitempty"`
	KeyFile                string            `yaml:"keyFile,omitempty"`
	Key                    string            `yaml:"key,omitempty"`
	InsecureSkipHostVerify bool              `yaml:"insecureSkipHostVerify"`
	RequiredDNAttributes   map[string]string `yaml:"requiredDNAttributes,omitempty"`
}
