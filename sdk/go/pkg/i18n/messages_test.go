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

package i18n

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"
)

var (
	TestError1  = PDE(language.AmericanEnglish, "PD02001", "Test error 1: %s")
	TestError2  = PDE(language.AmericanEnglish, "PD02002", "Test error 2: %s")
	TestError3  = PDE(language.AmericanEnglish, "PD02003", "Test error 3: %s", 400)
	TestConfig1 = PDC(language.AmericanEnglish, "config.something.1", "Test config field 1", "some type")

	TestError1Lang2  = PDE(language.Spanish, "PD02001", "Error de prueba 1: %s")
	TestConfig1Lang2 = PDC(language.Spanish, "config.something.1", "campo de configuración de prueba", "some type")
)

func TestExpand(t *testing.T) {
	ctx := WithLang(context.Background(), language.AmericanEnglish)
	str := Expand(ctx, MessageKey(TestError1), "myinsert")
	assert.Equal(t, "Test error 1: myinsert", str)
}

func TestExpandNoLangContext(t *testing.T) {
	ctx := context.Background()
	str := Expand(ctx, MessageKey(TestError1), "myinsert")
	assert.Equal(t, "Test error 1: myinsert", str)
}

func TestExpandNoLangContextLang2(t *testing.T) {
	ctx := context.Background()
	SetLang("es")
	str := Expand(ctx, MessageKey(TestError1), "myinsert")
	assert.Equal(t, "Error de prueba 1: myinsert", str)
}

func TestExpandNoLangContextLang2Fallback(t *testing.T) {
	ctx := context.Background()
	SetLang("es")
	str := Expand(ctx, MessageKey(TestError2), "myinsert")
	assert.Equal(t, "Test error 2: myinsert", str)
}

func TestExpandLanguageFallback(t *testing.T) {
	ctx := WithLang(context.Background(), language.Spanish)
	str := Expand(ctx, MessageKey(TestError2), "myinsert")
	assert.Equal(t, "Test error 2: myinsert", str)
}

func TestExpandWithCode(t *testing.T) {
	ctx := WithLang(context.Background(), language.AmericanEnglish)
	str := ExpandWithCode(ctx, MessageKey(TestError2), "myinsert")
	assert.Equal(t, "PD02002: Test error 2: myinsert", str)
}

func TestExpandWithCodeLangaugeFallback(t *testing.T) {
	ctx := WithLang(context.Background(), language.Spanish)
	str := ExpandWithCode(ctx, MessageKey(TestError2), "myinsert")
	assert.Equal(t, "PD02002: Test error 2: myinsert", str)
}

func TestExpandWithCodeLang2(t *testing.T) {
	ctx := WithLang(context.Background(), language.Spanish)
	str := ExpandWithCode(ctx, MessageKey(TestError1), "myinsert")
	assert.Equal(t, "PD02001: Error de prueba 1: myinsert", str)
}

func TestGetStatusHint(t *testing.T) {
	code, ok := GetStatusHint(string(TestError3))
	assert.True(t, ok)
	assert.Equal(t, 400, code)
}

func TestDuplicateKey(t *testing.T) {
	PDM(language.AmericanEnglish, "FF109999", "test1")
	assert.Panics(t, func() {
		PDM(language.AmericanEnglish, "FF109999", "test2")
	})
}

func TestInvalidPrefixKey(t *testing.T) {
	assert.Panics(t, func() {
		PDE(language.AmericanEnglish, "ABCD1234", "test1")
	})
}

func TestConfigMessageKey(t *testing.T) {
	ctx := WithLang(context.Background(), language.AmericanEnglish)
	str := Expand(ctx, MessageKey(TestConfig1))
	assert.Equal(t, "Test config field 1", str)
}

func TestConfigMessageKeyLang2(t *testing.T) {
	ctx := WithLang(context.Background(), language.Spanish)
	str := Expand(ctx, MessageKey(TestConfig1))
	assert.Equal(t, "campo de configuración de prueba", str)
}

func TestGetFieldType(t *testing.T) {
	fieldType, ok := GetFieldType(string(TestConfig1))
	assert.True(t, ok)
	assert.Equal(t, "some type", fieldType)
}

func TestDuplicateConfigKey(t *testing.T) {
	PDC(language.AmericanEnglish, "config.test.2", "test2 description", "type")
	assert.Panics(t, func() {
		PDC(language.AmericanEnglish, "config.test.2", "test2 dupe", "dupe type")
	})
}

func TestRegisterPrefixOK(t *testing.T) {
	RegisterPrefix("AB12", "my microservice")
	msgMyMessage := PDE(language.AmericanEnglish, "AB1200000", "Something went pop")
	err := NewError(context.Background(), msgMyMessage)
	assert.Regexp(t, "AB1200000", err)
}

func TestRegisterPrefixInvalid(t *testing.T) {
	assert.Panics(t, func() {
		RegisterPrefix("wrong", "my microservice")
	})
}

func TestRegisterPrefixFF(t *testing.T) {
	assert.Panics(t, func() {
		RegisterPrefix("wrong", "FF01")
	})
}

func TestRegisterPrefixDuplicate(t *testing.T) {
	assert.Panics(t, func() {
		RegisterPrefix("AB12", "my microservice")
		RegisterPrefix("AB12", "my microservice")
	})
}
