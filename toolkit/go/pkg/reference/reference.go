// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reference

import (
	"context"
)

// GenerateAllReferenceMarkdown generates both API and config documentation
func GenerateAllReferenceMarkdown(ctx context.Context) (map[string][]byte, error) {
	// Generate API documentation
	apiDocs, err := GenerateAPIObjectsReferenceMarkdown(ctx)
	if err != nil {
		return nil, err
	}

	// Generate config documentation
	configDocs, err := GenerateConfigReferenceMarkdown(ctx)
	if err != nil {
		return nil, err
	}

	// Merge the maps
	allDocs := make(map[string][]byte)
	for k, v := range apiDocs {
		allDocs[k] = v
	}
	for k, v := range configDocs {
		allDocs[k] = v
	}

	return allDocs, nil
}
