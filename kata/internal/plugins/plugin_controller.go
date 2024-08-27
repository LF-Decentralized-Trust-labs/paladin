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
package plugins

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
)

type Managers interface {
	DomainRegistration() DomainRegistration
}

type PluginController interface {
	Start() error
	Stop()
	GRPCTargetURL() string
	LoaderID() uuid.UUID
	WaitForInit(ctx context.Context) error
	ReloadPluginList() error
}

type pluginController struct {
	prototk.UnimplementedPluginControllerServer
	bgCtx    context.Context
	mux      sync.Mutex
	listener net.Listener
	server   *grpc.Server

	loaderID        uuid.UUID
	network         string
	address         string
	shutdownTimeout time.Duration

	domainManager DomainRegistration
	domainPlugins map[uuid.UUID]*plugin[prototk.DomainMessage]

	notifyPluginsUpdated chan bool
	pluginLoaderDone     chan struct{}
	loadingProgressed    chan *prototk.PluginLoadFailed
	serverDone           chan error
}

func NewPluginController(bgCtx context.Context,
	socketFile string, // default is a UDS path, can use tcp:127.0.0.1:12345 strings too (or tcp4:/tcp6:)
	loaderID uuid.UUID,
	managers Managers,
	conf *PluginControllerConfig) (_ PluginController, err error) {

	pc := &pluginController{
		bgCtx: bgCtx,

		loaderID:        loaderID,
		shutdownTimeout: confutil.DurationMin(conf.GRPC.ShutdownTimeout, 0, *DefaultGRPCConfig.ShutdownTimeout),

		domainManager: managers.DomainRegistration(),
		domainPlugins: make(map[uuid.UUID]*plugin[prototk.DomainMessage]),

		serverDone:           make(chan error),
		notifyPluginsUpdated: make(chan bool, 1),
		loadingProgressed:    make(chan *prototk.PluginLoadFailed, 1),
	}

	if err := pc.ReloadPluginList(); err != nil {
		return nil, err
	}

	if err := pc.parseGRPCAddress(bgCtx, socketFile); err != nil {
		return nil, err
	}
	return pc, nil
}

func (pc *pluginController) parseGRPCAddress(ctx context.Context, serverAddr string) error {

	// We support a subset of the Go network prefixes, and if none is found, we default to UNIX Domain Sockets ("unix:")
	tcpStr, isTCP := strings.CutPrefix(serverAddr, "tcp:")
	if isTCP {
		pc.network = "tcp"
		pc.address = tcpStr
		return nil
	}
	tcp4Str, isTCP4 := strings.CutPrefix(serverAddr, "tcp4:")
	if isTCP4 {
		pc.network = "tcp4"
		pc.address = tcp4Str
		return nil
	}
	tcp6Str, isTCP6 := strings.CutPrefix(serverAddr, "tcp6:")
	if isTCP6 {
		pc.network = "tcp6"
		pc.address = tcp6Str
		return nil
	}
	udsPath := strings.Trim(serverAddr, "unix:")
	if len(udsPath) > 107 {
		// socket paths longer than 107 are not safe for UDS on Linux/Mac and will fail with an odd bind error.
		// So we fail with a hard to explain bind failure - so we give a nicer error here
		// Note: it's not actually 107 chars on all platforms, but it's good enough for us.
		// See https://man7.org/linux/man-pages/man7/unix.7.html for a complex description of "sun_path"
		return i18n.NewError(ctx, msgs.MsgPluginUDSPathTooLong, len(udsPath))
	}
	pc.network = "unix"
	pc.address = udsPath
	return nil

}

func (pc *pluginController) Start() (err error) {
	ctx := pc.bgCtx
	log.L(ctx).Infof("server starting on %s:%s", pc.network, pc.address)
	pc.listener, err = net.Listen(pc.network, pc.address)
	if err != nil {
		log.L(ctx).Error("failed to listen: ", err)
		return err
	}
	pc.server = grpc.NewServer()

	prototk.RegisterPluginControllerServer(pc.server, pc)

	log.L(ctx).Infof("server listening at %v", pc.listener.Addr())
	go pc.runServer(ctx)
	return nil
}

func (pc *pluginController) runServer(ctx context.Context) {
	log.L(ctx).Infof("Run GRPC Server")

	log.L(ctx).Infof("Server started")
	pc.serverDone <- pc.server.Serve(pc.listener)
	log.L(ctx).Infof("Server ended")
}

func (pc *pluginController) Stop() {
	ctx := pc.bgCtx
	log.L(ctx).Infof("Stop")

	gracefullyStopped := make(chan struct{})
	go func() {
		defer close(gracefullyStopped)
		pc.server.GracefulStop()
	}()
	select {
	case <-gracefullyStopped:
	case <-time.After(pc.shutdownTimeout):
		pc.server.Stop()
	}
	serverErr := <-pc.serverDone
	log.L(ctx).Infof("Server stopped (err=%v)", serverErr)

}

// Must be started to call this function
func (pc *pluginController) GRPCTargetURL() string {
	switch pc.network {
	case "unix":
		return "unix:" + pc.listener.Addr().String()
	default:
		return "dns:///" + pc.listener.Addr().String()
	}
}

func (pc *pluginController) LoaderID() uuid.UUID {
	return pc.loaderID
}

func (pc *pluginController) ReloadPluginList() error {
	for name, dp := range pc.domainManager.ConfiguredDomains() {
		if err := initPlugin(pc.bgCtx, pc, pc.domainPlugins, name, prototk.PluginInfo_DOMAIN, dp); err != nil {
			return err
		}
	}
	select {
	case pc.notifyPluginsUpdated <- true:
	default:
	}
	return nil
}

func (pc *pluginController) WaitForInit(ctx context.Context) error {
	for {
		unloadedDomainPlugins, _ := unloadedPlugins(pc, pc.domainPlugins, prototk.PluginInfo_DOMAIN, false)
		unloadedCount := len(unloadedDomainPlugins)
		if unloadedCount == 0 {
			return nil
		}
		select {
		case loadErrOrNil := <-pc.loadingProgressed:
			if loadErrOrNil != nil {
				return i18n.NewError(ctx, msgs.MsgPluginLoadFailed, loadErrOrNil.ErrorMessage)
			}
		case <-ctx.Done():
			log.L(ctx).Warnf("server exiting before plugin initialization complete")
			return i18n.NewError(ctx, msgs.MsgContextCanceled)
		}
	}
}

func (pc *pluginController) newReqContext() context.Context {
	return log.WithLogField(pc.bgCtx, "plugin_reqid", types.ShortID())
}

func (pc *pluginController) InitLoader(req *prototk.PluginLoaderInit, stream prototk.PluginController_InitLoaderServer) error {
	ctx := pc.newReqContext()
	suppliedID, err := uuid.Parse(req.Id)
	if err != nil || suppliedID != pc.loaderID {
		return i18n.WrapError(ctx, err, msgs.MsgPluginLoaderUUIDError)
	}
	pc.mux.Lock()
	if pc.pluginLoaderDone != nil {
		pc.mux.Unlock()
		return i18n.WrapError(ctx, err, msgs.MsgPluginLoaderAlreadyInit)
	}
	pc.pluginLoaderDone = make(chan struct{})
	pc.mux.Unlock()
	log.L(ctx).Infof("Plugin loader connected")
	return pc.sendPluginsToLoader(stream)
}

func (pc *pluginController) LoadFailed(ctx context.Context, req *prototk.PluginLoadFailed) (*prototk.EmptyResponse, error) {
	log.L(ctx).Errorf("Plugin load %s (type=%s) failed: %s", req.Plugin.Name, req.Plugin.PluginType, req.ErrorMessage)
	select {
	case pc.loadingProgressed <- req:
	default:
	}
	return &prototk.EmptyResponse{}, nil
}

func initPlugin[CB any](ctx context.Context, pc *pluginController, pluginMap map[uuid.UUID]*plugin[CB], name string, pType prototk.PluginInfo_PluginType, conf *PluginConfig) (err error) {
	pc.mux.Lock()
	defer pc.mux.Unlock()
	plugin := &plugin[CB]{pc: pc, id: uuid.New(), name: name}
	if err := types.ValidateSafeCharsStartEndAlphaNum(ctx, name, types.DefaultNameMaxLen, "name"); err != nil {
		return err
	}
	plugin.def = &prototk.PluginLoad{
		Plugin: &prototk.PluginInfo{
			Id:         plugin.id.String(),
			Name:       name,
			PluginType: pType,
		},
		LibLocation: conf.Library,
		Class:       conf.Class,
	}
	plugin.def.LibType, err = types.MapEnum(conf.Type, golangToProtoLibTypeMap)
	pluginMap[plugin.id] = plugin
	return err
}

func (pc *pluginController) tapLoadingProgressed() {
	select {
	case pc.loadingProgressed <- nil:
	default:
	}
}

func unloadedPlugins[CB any](pc *pluginController, pluginMap map[uuid.UUID]*plugin[CB], pbType prototk.PluginInfo_PluginType, setInitializing bool) (unloaded, notInitializing []*plugin[CB]) {
	pc.mux.Lock()
	defer pc.mux.Unlock()
	pluginList := []string{}
	for name, plugin := range pluginMap {
		pluginList = append(pluginList, fmt.Sprintf("%s:%s", plugin.def.Plugin.PluginType, name))
		if !plugin.initialized {
			if !plugin.initializing {
				notInitializing = append(notInitializing, plugin)
				if setInitializing {
					plugin.initializing = true
				}
			}
			unloaded = append(unloaded, plugin)
		}
	}
	if len(unloaded) > 0 {
		log.L(pc.bgCtx).Debugf("%d of %d %s plugins loaded", len(pluginMap)-len(unloaded), len(pluginMap), pbType)
	} else {
		log.L(pc.bgCtx).Infof("All plugins loaded %v", pluginList)
	}
	return unloaded, notInitializing
}

func getPluginByIDString[CB any](pc *pluginController, pluginMap map[uuid.UUID]*plugin[CB], idStr string, pbType prototk.PluginInfo_PluginType) (p *plugin[CB], err error) {
	pc.mux.Lock()
	defer pc.mux.Unlock()
	id, err := uuid.Parse(idStr)
	if err == nil {
		p = pluginMap[id]
		if p == nil {
			err = i18n.NewError(pc.bgCtx, msgs.MsgPluginUUIDNotFound, pbType, id)
		}
	}
	return p, err
}

func (pc *pluginController) sendPluginsToLoader(stream prototk.PluginController_InitLoaderServer) (err error) {
	defer func() {
		pc.mux.Lock()
		defer pc.mux.Unlock()
		close(pc.pluginLoaderDone)
		pc.pluginLoaderDone = nil
	}()
	ctx := stream.Context()
	for {
		// We send a load request for each plugin that isn't new - which should result in that plugin being loaded
		// and resulting in a ConnectDomain bi-directional stream being set up.
		_, notInitializing := unloadedPlugins(pc, pc.domainPlugins, prototk.PluginInfo_DOMAIN, true)
		for _, plugin := range notInitializing {
			if err == nil {
				err = stream.Send(plugin.def)
			}
		}
		if err == nil {
			select {
			case <-ctx.Done():
				log.L(ctx).Debugf("loader stream closed")
				err = i18n.NewError(ctx, msgs.MsgContextCanceled)
			case <-pc.notifyPluginsUpdated:
				// loop and load any that need loading
			}
		}
		if err != nil {
			return err
		}
	}
}
