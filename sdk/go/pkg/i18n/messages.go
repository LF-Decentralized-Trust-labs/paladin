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
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/message/catalog"
)

// MessageKey is the lookup string for any translation
type MessageKey string

// ErrorMessageKey is a special lookup string conforming to Paladin's rules for error message registration
type ErrorMessageKey MessageKey

// ConfigMessageKey is a special lookup string conforming to Paladin's rules for configuration descriptions and types
type ConfigMessageKey MessageKey

// Expand for use in docs and logging - returns a translated message, translated the language of the context
// If a translation is not found for the language of the context, the default language text will be returned instead
func Expand(ctx context.Context, key MessageKey, inserts ...interface{}) string {
	translation := pFor(ctx).Sprintf(string(key), inserts...)
	if translation != string(key) {
		return translation
	}
	return fallbackLangPrinter.Sprintf(string(key), inserts...)
}

// ExpandWithCode for use in error scenarios - returns a translated message with a "MSG012345:" prefix, translated the language of the context
func ExpandWithCode(ctx context.Context, key MessageKey, inserts ...interface{}) string {
	translation := string(key) + ": " + pFor(ctx).Sprintf(string(key), inserts...)
	if translation != string(key)+": "+string(key) {
		return translation
	}
	return string(key) + ": " + fallbackLangPrinter.Sprintf(string(key), inserts...)
}

// WithLang sets the language on the context
func WithLang(ctx context.Context, lang language.Tag) context.Context {
	return context.WithValue(ctx, ctxLangKey{}, lang)
}

type (
	ctxLangKey struct{}
)

var statusHints = map[string]int{}
var fieldTypes = map[string]string{}
var msgIDUniq = map[string]bool{}

var fallbackLangPrinter = message.NewPrinter(language.AmericanEnglish)

var prefixValidator = regexp.MustCompile(`[A-Z][A-Z]\d\d`)

// RegisterPrefix allows components to register a message prefix that is not known to the core
// Paladin codebase. The toolkit does not provide the clash-protection
// for these prefixes - so if you're writing a new component within Paladin
// then you should instead submit a PR to update the registeredPrefixes constant.
//
// If you're writing a proprietary extension, or just using the Microservice framework for your
// own purposes, then you can register your own prefix with this function dynamically.
//
// It must conform to being two upper case characters, then two numbers.
// You cannot use the `PD` prefix characters, as they are reserved for Paladin components
func RegisterPrefix(prefix, description string) {
	if !prefixValidator.MatchString(prefix) || strings.HasPrefix(prefix, "FF") {
		panic(fmt.Sprintf("invalid prefix %q", prefix))
	}
	if _, ok := registeredPrefixes[prefix]; ok {
		panic(fmt.Sprintf("duplicate prefix %q", prefix))
	}
	registeredPrefixes[prefix] = description
}

// PDE is the translation helper to register an error message
func PDE(language language.Tag, key, translation string, statusHint ...int) ErrorMessageKey {
	validPrefix := false
	for rp := range registeredPrefixes {
		if strings.HasPrefix(key, rp) {
			validPrefix = true
			break
		}
	}
	if !validPrefix {
		panic(fmt.Sprintf("Message ID %s does not use one of the registered prefixes in the common utility package", key))
	}
	msgID := PDM(language, key, translation)
	if len(statusHint) > 0 {
		statusHints[key] = statusHint[0]
	}
	return ErrorMessageKey(msgID)
}

// PDM is the translation helper to define a new message (not used in translation files)
func PDM(language language.Tag, key, translation string) MessageKey {
	if checkKeyExists(language, key) {
		panic(fmt.Sprintf("Message ID %s re-used", key))
	}
	setKeyExists(language, key)
	_ = message.Set(language, key, catalog.String(translation))
	return MessageKey(key)
}

// FFC is the translation helper to define a configuration key description and type
func PDC(language language.Tag, key, translation, fieldType string) ConfigMessageKey {
	if checkKeyExists(language, key) {
		panic(fmt.Sprintf("Config ID %s re-used", key))
	}
	setKeyExists(language, key)
	fieldTypes[key] = fieldType
	_ = message.Set(language, key, catalog.String(translation))
	return ConfigMessageKey(key)
}

var defaultLangPrinter *message.Printer

func pFor(ctx context.Context) *message.Printer {
	lang := ctx.Value(ctxLangKey{})
	if lang == nil {
		return defaultLangPrinter
	}
	return message.NewPrinter(lang.(language.Tag))
}

func init() {
	SetLang("en")
	msgIDUniq = map[string]bool{} // Clear out that memory as no longer needed
}

func SetLang(lang string) {
	// Allow a lang var to be used
	tag := message.MatchLanguage(lang)
	defaultLangPrinter = message.NewPrinter(tag)
}

func GetStatusHint(code string) (int, bool) {
	i, ok := statusHints[code]
	return i, ok
}

func GetFieldType(code string) (string, bool) {
	s, ok := fieldTypes[code]
	return s, ok
}

func setKeyExists(language language.Tag, key string) {
	msgIDUniq[fmt.Sprintf("%s_%s", language, key)] = true
}

func checkKeyExists(language language.Tag, key string) bool {
	return msgIDUniq[fmt.Sprintf("%s_%s", language, key)]
}
