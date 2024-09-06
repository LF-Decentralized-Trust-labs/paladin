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

package componentmgr

import (
	"context"
	"os"
	"testing"

	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAndParseYAMLFileFlatStruct(t *testing.T) {
	ctx := context.Background()

	type testConfigType struct {
		Foo *string `yaml:"foo"`
		Bar *int    `yaml:"bar"`
		Baz *int    `yaml:"baz"`
	}
	// Create a temporary test file
	tempFile, err := os.CreateTemp("", "test_*.yaml")
	require.NoError(t, err)

	defer os.Remove(tempFile.Name())

	// Write YAML content to the temporary file
	yamlContent := []byte(`
foo: value1
bar: 123
`)
	_, err = tempFile.Write(yamlContent)
	require.NoError(t, err)

	tempFile.Close()

	expectedResult := testConfigType{
		Foo: confutil.P("value1"),
		Bar: confutil.P(123),
	}

	result := testConfigType{}

	err = ReadAndParseYAMLFile(ctx, tempFile.Name(), &result)
	require.NoError(t, err)
	require.NotNil(t, result.Foo)
	assert.Equal(t, *expectedResult.Foo, *result.Foo)
	require.NotNil(t, result.Bar)
	assert.Equal(t, *expectedResult.Bar, *result.Bar)
	assert.Nil(t, result.Baz)
}

func TestReadAndParseYAMLFileNestedStruct(t *testing.T) {
	ctx := context.Background()

	type testConfigChildType struct {
		Foo *string `yaml:"foo"`
		Bar *int    `yaml:"bar"`
		Baz *int    `yaml:"baz"`
	}
	type testConfigType struct {
		Child *testConfigChildType `yaml:"child"`
		Baz   *int                 `yaml:"baz"`
	}
	// Create a temporary test file
	tempFile, err := os.CreateTemp("", "test_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	// Write YAML content to the temporary file
	yamlContent := []byte(`
child:
  foo: value1
  bar: 123
baz: 456
`)
	_, err = tempFile.Write(yamlContent)
	require.NoError(t, err)

	tempFile.Close()

	expectedResult := testConfigType{
		Child: &testConfigChildType{
			Foo: confutil.P("value1"),
			Bar: confutil.P(123),
		},
		Baz: confutil.P(456),
	}

	result := testConfigType{}

	err = ReadAndParseYAMLFile(ctx, tempFile.Name(), &result)
	require.NoError(t, err)
	require.NotNil(t, result.Child)
	require.NotNil(t, result.Child.Foo)
	assert.Equal(t, *expectedResult.Child.Foo, *result.Child.Foo)
	require.NotNil(t, result.Child.Bar)
	assert.Equal(t, *expectedResult.Child.Bar, *result.Child.Bar)
	require.NotNil(t, result.Baz)
	assert.Equal(t, *expectedResult.Baz, *result.Baz)
}

func TestReadAndParseYAMLFileNestedInlineStruct(t *testing.T) {
	ctx := context.Background()

	type testConfigChildType struct {
		Foo *string `yaml:"foo"`
		Bar *int    `yaml:"bar"`
		Baz *int    `yaml:"baz"`
	}

	type testConfigChildWrapperType struct {
		testConfigChildType `yaml:",inline"`
	}

	type testConfigType struct {
		Child *testConfigChildWrapperType `yaml:"child"`
		Baz   *int                        `yaml:"baz"`
	}
	// Create a temporary test file
	tempFile, err := os.CreateTemp("", "test_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	// Write YAML content to the temporary file
	yamlContent := []byte(`
child:
  foo: value1
  bar: 123
baz: 456
`)
	_, err = tempFile.Write(yamlContent)
	require.NoError(t, err)

	tempFile.Close()

	expectedResult := testConfigType{
		Child: &testConfigChildWrapperType{},
	}
	expectedResult.Child.Foo = confutil.P("value1")
	expectedResult.Child.Bar = confutil.P(123)
	expectedResult.Baz = confutil.P(456)

	result := testConfigType{}

	err = ReadAndParseYAMLFile(ctx, tempFile.Name(), &result)
	require.NoError(t, err)
	require.NotNil(t, result.Child)
	require.NotNil(t, result.Child.Foo)
	assert.Equal(t, *expectedResult.Child.Foo, *result.Child.Foo)
	require.NotNil(t, result.Child.Bar)
	assert.Equal(t, *expectedResult.Child.Bar, *result.Child.Bar)
	require.NotNil(t, result.Baz)
	assert.Equal(t, *expectedResult.Baz, *result.Baz)
}

func TestReadAndParseYAMLFileFailMissingFile(t *testing.T) {

	ctx := context.Background()
	// Create a temporary test file
	tempFile, err := os.CreateTemp("", "test_*.yaml")
	require.NoError(t, err)

	//remove the file imemdiately
	// we only need the name
	os.Remove(tempFile.Name())

	tempFile.Close()

	err = ReadAndParseYAMLFile(ctx, tempFile.Name(), confutil.P(struct{}{}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PD020200")
	assert.Contains(t, err.Error(), tempFile.Name())

}

func TestReadAndParseYAMLFileFailDirNotFile(t *testing.T) {
	ctx := context.Background()
	err := ReadAndParseYAMLFile(ctx, t.TempDir(), confutil.P(struct{}{}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PD020201")
}

func TestReadAndParseYAMLFileFailedParse(t *testing.T) {
	ctx := context.Background()

	type testConfigType struct {
		Foo *string `yaml:"foo"`
		Bar *int    `yaml:"bar"`
		Baz *int    `yaml:"baz"`
	}
	// Create a temporary test file
	tempFile, err := os.CreateTemp("", "test_*.yaml")
	require.NoError(t, err)

	defer os.Remove(tempFile.Name())

	// Write YAML content to the temporary file
	yamlContent := []byte(`
foo: value1
bar: 123
invalid yaml content
`)
	_, err = tempFile.Write(yamlContent)
	require.NoError(t, err)
	tempFile.Close()

	result := testConfigType{}

	err = ReadAndParseYAMLFile(ctx, tempFile.Name(), &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PD020202")
}
