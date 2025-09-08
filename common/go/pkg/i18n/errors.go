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
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

func truncate(s string, limit int) string {
	if len(s) > limit {
		return s[0:limit-3] + "..."
	}
	return s
}

type PDError interface {
	error
	MessageKey() ErrorMessageKey
	HTTPStatus() int
	StackTrace() string
}

type stackTracer interface {
	StackTrace() errors.StackTrace
}

type pdError struct {
	error
	msgKey ErrorMessageKey
	status int
}

func (ffe *pdError) MessageKey() ErrorMessageKey {
	return ffe.msgKey
}

func (ffe *pdError) HTTPStatus() int {
	return ffe.status
}

func (ffe *pdError) StackTrace() string {
	if st, ok := interface{}(ffe.error).(stackTracer); ok {
		buff := new(strings.Builder)
		for _, frame := range st.StackTrace() {
			fmt.Fprintf(buff, "%+v\n", frame)
		}
		return buff.String()
	}
	return ""
}

func pdWrap(err error, msgKey ErrorMessageKey) error {
	status, ok := statusHints[string(msgKey)]
	if !ok {
		status = http.StatusInternalServerError
	}
	return &pdError{
		error:  err,
		msgKey: msgKey,
		status: status,
	}
}

// NewError creates a new error
func NewError(ctx context.Context, msg ErrorMessageKey, inserts ...interface{}) error {
	return pdWrap(errors.New(truncate(ExpandWithCode(ctx, MessageKey(msg), inserts...), 2048)), msg)
}

// WrapError wraps an error
func WrapError(ctx context.Context, err error, msg ErrorMessageKey, inserts ...interface{}) error {
	if err == nil {
		return NewError(ctx, msg, inserts...)
	}
	return pdWrap(errors.Wrap(err, truncate(ExpandWithCode(ctx, MessageKey(msg), inserts...), 2048)), msg)
}
