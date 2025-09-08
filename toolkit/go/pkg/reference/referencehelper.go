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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

func getIncludeFile(ctx context.Context, outputPath, name string) ([]byte, error) {
	// If a detailed type_description.md file exists, include that in a Description section here
	filename, err := filepath.Abs(filepath.Join(outputPath, "_includes", fmt.Sprintf("%s_description.md", name)))
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filename); err != nil {
		return nil, i18n.NewError(ctx, pldmsgs.MsgReferenceMarkdownMissing, filename)
	}
	return []byte(fmt.Sprintf("{%% include-markdown \"./_includes/%s_description.md\" %%}\n\n", name)), nil
}

// shouldFilter checks if a type should be ignored (e.g., context.Context or error).
func shouldFilter(t reflect.Type) bool {
	return t == reflect.TypeOf((*context.Context)(nil)).Elem() ||
		t == reflect.TypeOf((*error)(nil)).Elem()
}

func isEnum(f reflect.Type) bool {
	return f.PkgPath() == "github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes" && strings.HasPrefix(f.Name(), "Enum[")
}

func generateEnumList(f reflect.Type) string {

	if f.Kind() == reflect.Pointer {
		f = f.Elem()
	}
	optionsMethod, _ := f.MethodByName("Options")
	enumOptions := optionsMethod.Func.Call([]reflect.Value{reflect.New(f).Elem()})[0].Interface().([]string)

	buff := new(strings.Builder)
	for i, v := range enumOptions {
		if i > 0 {
			buff.WriteString(", ")
		}
		fmt.Fprintf(buff, `"%s"`, v)
	}
	return buff.String()
}

func getRelativePath(depth int) string {
	path := filepath.Join("doc-site", "docs", "reference")
	for i := 0; i < depth; i++ {
		path = filepath.Join("..", path)
	}
	return path
}
func getType(v interface{}) reflect.Type {
	if reflect.TypeOf(v).Kind() == reflect.Ptr {
		return reflect.TypeOf(v).Elem()
	}
	if reflect.TypeOf(v).Kind() == reflect.Interface {
		return reflect.TypeOf(v).Elem()
	}
	return reflect.TypeOf(v)
}
