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

package pldresty

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/common/go/pkg/pldmsgs"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/tlsconf"
	"github.com/sirupsen/logrus"
)

type retryCtxKey struct{}
type hostCtxKey struct{}

type retryCtx struct {
	id       string
	start    time.Time
	attempts uint
}

// OnAfterResponse when using SetDoNotParseResponse(true) for streaming binary replies,
// the caller should invoke ffresty.OnAfterResponse on the response manually.
// The middleware is disabled on this path :-(
// See: https://github.com/go-resty/resty/blob/d01e8d1bac5ba1fed0d9e03c4c47ca21e94a7e8e/client.go#L912-L948
func OnAfterResponse(c *resty.Client, resp *resty.Response) {
	if c == nil || resp == nil {
		return
	}
	rCtx := resp.Request.Context()
	rc := rCtx.Value(retryCtxKey{}).(*retryCtx)
	level := logrus.DebugLevel
	status := resp.StatusCode()
	if status >= 300 {
		level = logrus.ErrorLevel
	}
	log.L(rCtx).Logf(level, "<== %s %s [%d] (%dms)", resp.Request.Method, resp.Request.URL, status, time.Since(rc.start).Milliseconds())
	// TODO use req.TraceInfo() for richer metrics at the DNS and transport layer
}

// New creates a new Resty client, using configuration that is passed in
//
// You can use the normal Resty builder pattern, to set per-instance configuration
// as required.
func New(ctx context.Context, conf *pldconf.HTTPClientConfig) (client *resty.Client, err error) { //nolint:gocyclo
	httpTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   time.Duration(confutil.DurationMin(conf.ConnectionTimeout, 0, *pldconf.DefaultHTTPConfig.ConnectionTimeout)),
			KeepAlive: time.Duration(confutil.DurationMin(conf.ConnectionTimeout, 0, *pldconf.DefaultHTTPConfig.ConnectionTimeout)),
		}).DialContext,
		ForceAttemptHTTP2: true,
	}

	u, err := url.Parse(conf.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, i18n.WrapError(ctx, err, pldmsgs.MsgRPCClientInvalidHTTPURL, u)
	}
	if u.Scheme == "https" {
		conf.TLS.Enabled = true
	}
	httpTransport.TLSClientConfig, err = tlsconf.BuildTLSConfig(ctx, &conf.TLS, tlsconf.ClientType)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: httpTransport,
	}
	client = resty.NewWithClient(httpClient)

	_url := strings.TrimSuffix(conf.URL, "/")
	if _url != "" {
		client.SetBaseURL(_url)
		log.L(ctx).Debugf("Created REST client to %s", _url)
	}

	client.SetTimeout(confutil.DurationMin(conf.RequestTimeout, 0, *pldconf.DefaultHTTPConfig.RequestTimeout))

	client.OnBeforeRequest(func(c *resty.Client, req *resty.Request) error {
		rCtx := req.Context()
		// Record host in context to avoid redundant parses in hooks
		var u *url.URL
		if req.URL != "" {
			u, _ = url.Parse(req.URL)
		}
		// The req.URL might have only set a path i.e. /home, fallbacking to the base URL of the client.
		// So if the URL is nil, that's likely the case and we'll derive the host from the configured
		// base instead.
		if (u == nil || u.Host == "") && c.BaseURL != "" {
			u, _ = url.Parse(c.BaseURL)
		}
		if u != nil && u.Host != "" {
			host := u.Host
			rCtx = context.WithValue(rCtx, hostCtxKey{}, host)
		}

		rc := rCtx.Value(retryCtxKey{})
		if rc == nil {
			// First attempt
			r := &retryCtx{
				id:    pldtypes.ShortID(),
				start: time.Now(),
			}
			rCtx = context.WithValue(rCtx, retryCtxKey{}, r)
			// Create a request logger from the root logger passed into the client
			rCtx = log.WithLogField(rCtx, "breq", r.id)
			req.SetContext(rCtx)
		}

		log.L(rCtx).Debugf("==> %s %s%s", req.Method, _url, req.URL)
		log.L(rCtx).Tracef("==> (body) %+v", req.Body)

		return nil
	})

	// Note that callers using SetNotParseResponse will need to invoke this themselves
	client.OnAfterResponse(func(c *resty.Client, r *resty.Response) error { OnAfterResponse(c, r); return nil })

	for k, v := range conf.HTTPHeaders {
		if vs, ok := v.(string); ok {
			client.SetHeader(k, vs)
		}
	}

	if conf.Auth.Username != "" && conf.Auth.Password != "" {
		client.SetHeader("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", conf.Auth.Username, conf.Auth.Password)))))
	}

	if conf.Retry.Enabled {
		var retryStatusCodeRegex *regexp.Regexp
		if conf.Retry.ErrorStatusCodes != "" {
			retryStatusCodeRegex = regexp.MustCompile(conf.Retry.ErrorStatusCodes)
		}

		retryCount := confutil.IntMin(conf.Retry.Count, 0, *pldconf.DefaultHTTPConfig.Retry.Count)
		minTimeout := time.Duration(confutil.DurationMin(conf.Retry.InitialDelay, 0, *pldconf.DefaultHTTPConfig.Retry.InitialDelay))
		maxTimeout := time.Duration(confutil.DurationMin(conf.Retry.MaximumDelay, 0, *pldconf.DefaultHTTPConfig.Retry.MaximumDelay))

		client.
			SetRetryCount(retryCount).
			SetRetryWaitTime(minTimeout).
			SetRetryMaxWaitTime(maxTimeout).
			AddRetryCondition(func(r *resty.Response, err error) bool {
				if r == nil || r.IsSuccess() {
					return false
				}

				if r.StatusCode() > 0 && retryStatusCodeRegex != nil && !retryStatusCodeRegex.MatchString(r.Status()) {
					// the error status code doesn't match the retry status code regex, stop retry
					return false
				}

				rCtx := r.Request.Context()
				rc := rCtx.Value(retryCtxKey{}).(*retryCtx)
				rc.attempts++
				log.L(rCtx).Infof("retry %d/%d (min=%dms/max=%dms) status=%d", rc.attempts, retryCount, minTimeout.Milliseconds(), maxTimeout.Milliseconds(), r.StatusCode())
				return true
			})
	}

	return client, nil
}

func WrapRestErr(ctx context.Context, res *resty.Response, err error, key i18n.ErrorMessageKey) error {
	var respData string
	if res != nil {
		if res.RawBody() != nil {
			defer func() { _ = res.RawBody().Close() }()
			if r, err := io.ReadAll(res.RawBody()); err == nil {
				respData = string(r)
			}
		}
		if respData == "" {
			respData = res.String()
		}
		if len(respData) > 256 {
			respData = respData[0:256] + "..."
		}
	}
	if err != nil {
		return i18n.WrapError(ctx, err, key, respData)
	}
	return i18n.NewError(ctx, key, respData)
}
