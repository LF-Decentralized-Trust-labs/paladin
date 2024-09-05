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

package rpcclient

import (
	"context"
	"net/url"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tlsconf"
)

type ConfigAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type HTTPConfig struct {
	URL         string                 `yaml:"url"`
	HTTPHeaders map[string]interface{} `yaml:"httpHeaders"`
	Auth        ConfigAuth             `yaml:"auth"`
	TLS         tlsconf.Config         `yaml:"tls"`
}

type WSConfig struct {
	HTTPConfig             `yaml:",inline"`
	InitialConnectAttempts *int         `yaml:"initialConnectAttempts"`
	ConnectionTimeout      *string      `yaml:"connectionTimeout"`
	ConnectRetry           retry.Config `yaml:"connectRetry"`
	ReadBufferSize         *string      `yaml:"readBufferSize"`
	WriteBufferSize        *string      `yaml:"writeBufferSize"`
	HeartbeatInterval      *string      `yaml:"heartbeatInterval"`
}

var DefaultWSConfig = &WSConfig{
	ReadBufferSize:         confutil.P("16Kb"),
	WriteBufferSize:        confutil.P("16Kb"),
	InitialConnectAttempts: confutil.P(0),
	ConnectionTimeout:      confutil.P("30s"),
	HeartbeatInterval:      confutil.P("15s"),
	ConnectRetry:           retry.Defaults.Config,
}

func ParseWSConfig(ctx context.Context, config *WSConfig) (*wsclient.WSConfig, error) {
	u, err := url.Parse(config.URL)
	if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") {
		return nil, i18n.WrapError(ctx, err, msgs.MsgEthClientInvalidWebSocketURL, u)
	}
	if u.Scheme == "wss" {
		config.TLS.Enabled = true
	}
	tlsConfig, err := tlsconf.BuildTLSConfig(ctx, &config.TLS, tlsconf.ClientType)
	if err != nil {
		return nil, err
	}
	return &wsclient.WSConfig{
		WebSocketURL:           u.String(),
		HTTPHeaders:            config.HTTPHeaders,
		ReadBufferSize:         int(confutil.ByteSize(config.ReadBufferSize, 0, *DefaultWSConfig.ReadBufferSize)),
		WriteBufferSize:        int(confutil.ByteSize(config.WriteBufferSize, 0, *DefaultWSConfig.WriteBufferSize)),
		ConnectionTimeout:      confutil.DurationMin(config.ConnectionTimeout, 0, *DefaultWSConfig.ConnectionTimeout),
		InitialDelay:           confutil.DurationMin(config.ConnectRetry.InitialDelay, 0, *DefaultWSConfig.ConnectRetry.InitialDelay),
		MaximumDelay:           confutil.DurationMin(config.ConnectRetry.MaxDelay, 0, *DefaultWSConfig.ConnectRetry.MaxDelay),
		HeartbeatInterval:      confutil.DurationMin(config.HeartbeatInterval, 0, *DefaultWSConfig.HeartbeatInterval),
		AuthUsername:           config.Auth.Username,
		AuthPassword:           config.Auth.Password,
		TLSClientConfig:        tlsConfig,
		InitialConnectAttempts: confutil.IntMin(config.InitialConnectAttempts, 0, *DefaultWSConfig.InitialConnectAttempts),
	}, nil
}

func ParseHTTPConfig(ctx context.Context, config *HTTPConfig) (*resty.Client, error) {
	u, err := url.Parse(config.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, i18n.WrapError(ctx, err, msgs.MsgEthClientInvalidHTTPURL, u)
	}
	if u.Scheme == "https" {
		config.TLS.Enabled = true
	}
	tlsConfig, err := tlsconf.BuildTLSConfig(ctx, &config.TLS, tlsconf.ClientType)
	if err != nil {
		return nil, err
	}
	restyConf := ffresty.Config{
		URL: u.String(),
		HTTPConfig: ffresty.HTTPConfig{
			HTTPHeaders:     config.HTTPHeaders,
			AuthUsername:    config.Auth.Username,
			AuthPassword:    config.Auth.Password,
			TLSClientConfig: tlsConfig,
		},
	}
	return ffresty.NewWithConfig(ctx, restyConf), nil
}
