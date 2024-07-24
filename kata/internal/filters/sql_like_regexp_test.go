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

package filters

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSQLLikeToRegexp(t *testing.T) {

	checkMapping := func(sqlLike, expectedRegex, test, negTest string) {
		r, err := sqlLikeToRegexp(sqlLike, false, '\\')
		assert.NoError(t, err)
		assert.Equal(t, expectedRegex, r.String())
		assert.True(t, r.MatchString(test))
		assert.False(t, r.MatchString(negTest))
	}

	checkMapping("%something%", "^.*?something.*?$", "lots of something stuff", "some thing")
	checkMapping("som_thing", "^som.thing$", "something", "someth1ng")
	checkMapping("s_______g", "^s.......g$", "smoothing", " smoothing ")
	checkMapping("\\%\\%%\\__\\_", "^%%.*?_._$", "%%stuff_A_", "stuff_A_")
	checkMapping("%\\\\%.thing", "^.*?\\\\.*?\\.thing$", "some\\stuff.thing", "somestuff.thing")
	checkMapping("(%)/(%).txt", "^\\(.*?\\)/\\(.*?\\)\\.txt$", "(some)/(thing).txt", "some/thing.txt")

}
