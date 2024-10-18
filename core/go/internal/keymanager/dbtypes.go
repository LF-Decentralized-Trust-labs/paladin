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

package keymanager

type DBKeyPath struct {
	Parent string `gorm:"column:parent;primaryKey"`
	Index  int64  `gorm:"column:index;primaryKey"`
	Path   string `gorm:"column:path"`
}

func (t DBKeyPath) TableName() string {
	return "key_paths"
}

type DBKeyMapping struct {
	Identifier string `gorm:"column:identifier;primaryKey"`
	Wallet     string `gorm:"column:wallet"`
	KeyHandle  string `gorm:"column:key_handle"`
}

func (t DBKeyMapping) TableName() string {
	return "key_mappings"
}

type DBKeyVerifier struct {
	Identifier string `gorm:"column:identifier;primaryKey"`
	Algorithm  string `gorm:"column:algorithm;primaryKey"`
	Type       string `gorm:"column:type;primaryKey"`
	Verifier   string `gorm:"column:verifier"`
}

func (t DBKeyVerifier) TableName() string {
	return "key_verifiers"
}
