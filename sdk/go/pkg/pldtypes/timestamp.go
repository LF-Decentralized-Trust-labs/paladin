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

package pldtypes

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"strconv"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

// Timestamp stores a Unix timestamp with nanoseconds.
// Timestamp is serialized to JSON on the API in RFC3339 nanosecond UTC time
// (noting that JavaScript can parse this format happily into millisecond time with Date.pase()).
// It is persisted as a nanosecond resolution timestamp in the database.
// It can be parsed from RFC3339, or unix timestamps (second, millisecond or nanosecond resolution)
type Timestamp int64

func TimestampNow() Timestamp {
	return Timestamp(time.Now().UnixNano())
}

func TimestampFromUnix(unixTime int64) Timestamp {
	if unixTime < 1e10 {
		unixTime *= 1e3 // secs to millis
	}
	if unixTime < 1e15 {
		unixTime *= 1e6 // millis to nanos
	}
	return Timestamp(unixTime)
}

func (ts *Timestamp) MarshalJSON() ([]byte, error) {
	if ts == nil || *ts == 0 {
		return json.Marshal(nil)
	}
	return json.Marshal(ts.String())
}

func ParseTimeString(str string) (Timestamp, error) {
	t, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		var unixTime int64
		unixTime, err = strconv.ParseInt(str, 10, 64)
		if err == nil {
			return TimestampFromUnix(unixTime), nil
		}
	}
	if err != nil {
		return 0, i18n.NewError(context.Background(), pldmsgs.MsgTypesTimeParseFail, str)
	}
	return Timestamp(t.UnixNano()), nil
}

func MustParseTimeString(str string) Timestamp {
	t, err := ParseTimeString(str)
	if err != nil {
		panic(err)
	}
	return t
}

func (ts Timestamp) Time() time.Time {
	return time.Unix(0, (int64)(ts))
}

func (ts Timestamp) UnixNano() int64 {
	return (int64)(ts)
}

func (ts *Timestamp) UnmarshalJSON(b []byte) error {
	var iVal interface{}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber() // It's not safe to use a JSON number decoder as it uses float64, so can (and does) lose precision
	err := decoder.Decode(&iVal)
	if err == nil {
		err = ts.Scan(iVal)
	}
	return err
}

func (ts *Timestamp) scanString(src string) error {
	t, err := ParseTimeString(src)
	if err != nil {
		return err
	}
	*ts = t
	return nil
}

// Scan implements sql.Scanner
func (ts *Timestamp) Scan(src interface{}) error {
	switch src := src.(type) {
	case nil:
		*ts = 0
		return nil
	case json.Number: // Note we avoid float64 using Decoder.UseNumber() in unmarshal
		return ts.scanString(src.String())
	case string:
		return ts.scanString(src)
	case int64:
		*ts = TimestampFromUnix(src)
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesRestoreFailed, src, ts)
	}

}

// Value implements sql.Valuer
func (ts Timestamp) Value() (driver.Value, error) {
	if ts == 0 {
		return int64(0), nil
	}
	return ts.UnixNano(), nil
}

func (ts Timestamp) String() string {
	if ts == 0 {
		return ""
	}
	return ts.Time().UTC().Format(time.RFC3339Nano)
}

func (ts *Timestamp) Equal(ts2 *Timestamp) bool {
	if ts == nil && ts2 == nil {
		return true
	}
	if ts == nil || ts2 == nil {
		return false
	}
	return *ts == *ts2
}
