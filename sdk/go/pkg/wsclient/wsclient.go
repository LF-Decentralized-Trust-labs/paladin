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

package wsclient

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/tlsconf"
	"github.com/gorilla/websocket"
)

type WSClient interface {
	Connect() error
	Receive() <-chan []byte
	URL() string
	SetURL(url string)
	SetHeader(header, value string)
	Send(ctx context.Context, message []byte) error
	Close()
}

type wsClient struct {
	ctx                  context.Context
	headers              http.Header
	url                  string
	initialRetryAttempts int
	wsdialer             *websocket.Dialer
	wsconn               *websocket.Conn
	retry                retry.Retry
	closed               bool
	receive              chan []byte
	send                 chan []byte
	sendDone             chan []byte
	closing              chan struct{}
	beforeConnect        WSPreConnectHandler
	afterConnect         WSPostConnectHandler
	heartbeatInterval    time.Duration
	heartbeatMux         sync.Mutex
	activePingSent       *time.Time
	lastPingCompleted    time.Time
}

// WSPreConnectHandler will be called before every connect/reconnect. Any error returned will prevent the websocket from connecting.
type WSPreConnectHandler func(ctx context.Context, w WSClient) error

// WSPostConnectHandler will be called after every connect/reconnect. Can send data over ws, but must not block listening for data on the ws.
type WSPostConnectHandler func(ctx context.Context, w WSClient) error

func New(ctx context.Context, config *pldconf.WSClientConfig, beforeConnect WSPreConnectHandler, afterConnect WSPostConnectHandler) (WSClient, error) {
	l := log.L(ctx)

	url, tlsConfig, err := ValidateConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	w := &wsClient{
		ctx: ctx,
		url: url.String(),
		wsdialer: &websocket.Dialer{
			ReadBufferSize:   int(confutil.ByteSize(config.ReadBufferSize, 0, *pldconf.DefaultWSConfig.ReadBufferSize)),
			WriteBufferSize:  int(confutil.ByteSize(config.WriteBufferSize, 0, *pldconf.DefaultWSConfig.WriteBufferSize)),
			TLSClientConfig:  tlsConfig,
			HandshakeTimeout: confutil.DurationMin(config.ConnectionTimeout, 0, *pldconf.DefaultWSConfig.ConnectionTimeout),
		},
		retry: *retry.NewRetryIndefinite(&pldconf.RetryConfig{
			InitialDelay: config.ConnectRetry.InitialDelay,
			MaxDelay:     config.ConnectRetry.MaxDelay,
		}),
		initialRetryAttempts: confutil.IntMin(config.InitialConnectAttempts, 0, *pldconf.DefaultWSConfig.InitialConnectAttempts),
		headers:              make(http.Header),
		send:                 make(chan []byte),
		closing:              make(chan struct{}),
		beforeConnect:        beforeConnect,
		afterConnect:         afterConnect,
		heartbeatInterval:    confutil.DurationMin(config.HeartbeatInterval, 0, *pldconf.DefaultWSConfig.HeartbeatInterval),
	}
	w.receive = make(chan []byte)

	for k, v := range config.HTTPHeaders {
		if vs, ok := v.(string); ok {
			w.headers.Set(k, vs)
		}
	}
	authUsername := config.Auth.Username
	authPassword := config.Auth.Password
	if authUsername != "" && authPassword != "" {
		w.headers.Set("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", authUsername, authPassword)))))
	}

	go func() {
		select {
		case <-ctx.Done():
			l.Tracef("WS %s closing due to canceled context", w.url)
			w.Close()
		case <-w.closing:
			l.Tracef("WS %s closing", w.url)
		}
	}()

	return w, nil
}

func ValidateConfig(ctx context.Context, config *pldconf.WSClientConfig) (*url.URL, *tls.Config, error) {
	u, err := url.Parse(config.URL)
	if err != nil || !strings.HasPrefix(u.Scheme, "ws") {
		return nil, nil, i18n.WrapError(ctx, err, pldmsgs.MsgWSClientInvalidWebSocketURL, config.URL)
	}

	if u.Scheme == "wss" {
		config.TLS.Enabled = true
	}
	tlsConfig, err := tlsconf.BuildTLSConfig(ctx, &config.TLS, tlsconf.ClientType)
	if err != nil {
		return nil, nil, err
	}
	return u, tlsConfig, err
}

func (w *wsClient) Connect() error {

	if err := w.connect(true); err != nil {
		return err
	}

	go w.receiveReconnectLoop()

	return nil
}

func (w *wsClient) Close() {
	if !w.closed {
		w.closed = true
		close(w.closing)
		c := w.wsconn
		if c != nil {
			_ = c.Close()
		}
	}
}

func (w *wsClient) Receive() <-chan []byte {
	return w.receive
}

func (w *wsClient) URL() string {
	return w.url
}

func (w *wsClient) SetURL(url string) {
	w.url = url
}

func (w *wsClient) SetHeader(header, value string) {
	w.headers.Set(header, value)
}

func (w *wsClient) Send(ctx context.Context, message []byte) error {
	// Send
	select {
	case w.send <- message:
		return nil
	case <-ctx.Done():
		return i18n.NewError(ctx, pldmsgs.MsgWSClientSendTimedOut)
	case <-w.closing:
		return i18n.NewError(ctx, pldmsgs.MsgWSClientClosing)
	}
}

func (w *wsClient) heartbeatTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if w.heartbeatInterval > 0 {
		w.heartbeatMux.Lock()
		baseTime := w.lastPingCompleted
		if w.activePingSent != nil {
			// We're waiting for a pong
			baseTime = *w.activePingSent
		}
		waitTime := w.heartbeatInterval - time.Since(baseTime) // if negative, will pop immediately
		w.heartbeatMux.Unlock()
		return context.WithTimeout(ctx, waitTime)
	}
	return context.WithCancel(ctx)
}

func (w *wsClient) connect(initial bool) error {
	l := log.L(w.ctx)
	return w.retry.Do(w.ctx, func(attempt int) (retry bool, err error) {
		if w.closed {
			return false, i18n.NewError(w.ctx, pldmsgs.MsgWSClientClosing)
		}

		retry = !initial || attempt < w.initialRetryAttempts
		if w.beforeConnect != nil {
			if err = w.beforeConnect(w.ctx, w); err != nil {
				l.Warnf("WS %s connect attempt %d failed in beforeConnect", w.url, attempt)
				return retry, err
			}
		}

		var res *http.Response
		w.wsconn, res, err = w.wsdialer.Dial(w.url, w.headers)
		if err != nil {
			var b []byte
			var status = -1
			if res != nil {
				b, _ = io.ReadAll(res.Body)
				_ = res.Body.Close()
				status = res.StatusCode
			}
			l.Warnf("WS %s connect attempt %d failed [%d]: %s", w.url, attempt, status, string(b))
			return retry, i18n.WrapError(w.ctx, err, pldmsgs.MsgWSClientConnectFailed)
		}

		w.pongReceivedOrReset(false)
		w.wsconn.SetPongHandler(w.pongHandler)
		l.Infof("WS %s connected", w.url)
		return false, nil
	})
}

func (w *wsClient) readLoop() {
	l := log.L(w.ctx)
	for {
		mt, message, err := w.wsconn.ReadMessage()
		if err != nil {
			// We treat this as informational, as it's normal for the client to disconnect here
			l.Infof("WS %s closed: %s", w.url, err)
			return
		}

		// Pass the message to the consumer
		l.Tracef("WS %s read (mt=%d): %s", w.url, mt, message)
		select {
		case <-w.sendDone:
			l.Debugf("WS %s closing reader after send error", w.url)
			return
		case w.receive <- message:
		}
	}
}

func (w *wsClient) pongHandler(_ string) error {
	w.pongReceivedOrReset(true)
	return nil
}

func (w *wsClient) pongReceivedOrReset(isPong bool) {
	w.heartbeatMux.Lock()
	defer w.heartbeatMux.Unlock()

	if isPong && w.activePingSent != nil {
		log.L(w.ctx).Debugf("WS %s heartbeat completed (pong) after %.2fms", w.url, float64(time.Since(*w.activePingSent))/float64(time.Millisecond))
	}
	w.lastPingCompleted = time.Now() // in new connection case we still want to consider now the time we completed the ping
	w.activePingSent = nil

	// We set a deadline for twice the heartbeat interval
	if w.heartbeatInterval > 0 {
		_ = w.wsconn.SetReadDeadline(time.Now().Add(2 * w.heartbeatInterval))
	}

}

func (w *wsClient) heartbeatCheck() error {
	w.heartbeatMux.Lock()
	defer w.heartbeatMux.Unlock()

	if w.activePingSent != nil {
		return i18n.NewError(w.ctx, pldmsgs.MsgWSClientHeartbeatTimeout, float64(time.Since(*w.activePingSent))/float64(time.Millisecond))
	}
	log.L(w.ctx).Debugf("WS %s heartbeat timer popped (ping) after %.2fms", w.url, float64(time.Since(w.lastPingCompleted))/float64(time.Millisecond))
	now := time.Now()
	w.activePingSent = &now
	return nil
}

func (w *wsClient) sendLoop(receiverDone chan struct{}) {
	l := log.L(w.ctx)
	defer close(w.sendDone)

	disconnecting := false
	for !disconnecting {
		timeoutContext, timeoutCancel := w.heartbeatTimeout(w.ctx)

		select {
		case message := <-w.send:
			l.Tracef("WS sending: %s", message)
			if err := w.wsconn.WriteMessage(websocket.TextMessage, message); err != nil {
				l.Errorf("WS %s send failed: %s", w.url, err)
				disconnecting = true
			}
		case <-timeoutContext.Done():
			wsconn := w.wsconn
			if err := w.heartbeatCheck(); err != nil {
				l.Errorf("WS %s closing: %s", w.url, err)
				disconnecting = true
			} else if wsconn != nil {
				if err := wsconn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					l.Errorf("WS %s heartbeat send failed: %s", w.url, err)
					disconnecting = true
				}
			}
		case <-receiverDone:
			l.Debugf("WS %s send loop exiting", w.url)
			disconnecting = true
		}

		timeoutCancel()
	}
}

func (w *wsClient) receiveReconnectLoop() {
	l := log.L(w.ctx)
	defer close(w.receive)
	for !w.closed {
		// Start the sender, letting it close without blocking sending a notification on the sendDone
		w.sendDone = make(chan []byte, 1)
		receiverDone := make(chan struct{})
		go w.sendLoop(receiverDone)

		// Call the reconnect processor
		var err error
		if w.afterConnect != nil {
			err = w.afterConnect(w.ctx, w)
		}

		if err == nil {
			// Synchronously invoke the reader, as it's important we react immediately to any error there.
			w.readLoop()
			close(receiverDone)
			<-w.sendDone

			// Ensure the connection is closed after the sender and receivers exit
			err = w.wsconn.Close()
			if err != nil {
				l.Debugf("WS %s close failed: %s", w.url, err)
			}
			w.sendDone = nil
			w.wsconn = nil
		}

		// Go into reconnect
		if !w.closed {
			err = w.connect(false)
			if err != nil {
				l.Debugf("WS %s exiting: %s", w.url, err)
				return
			}
		}
	}
}
