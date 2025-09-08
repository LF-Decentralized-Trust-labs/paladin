/*
 * Copyright © 2025 Kaleido, Inc.
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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
)

func MapLibraryTypeToProto(t pldtypes.Enum[pldtypes.LibraryType]) (prototk.PluginLoad_LibType, error) {
	return pldtypes.MapEnum(t, map[pldtypes.LibraryType]prototk.PluginLoad_LibType{
		pldtypes.LibraryTypeCShared: prototk.PluginLoad_C_SHARED,
		pldtypes.LibraryTypeJar:     prototk.PluginLoad_JAR,
	})
}

type pluginManager struct {
	prototk.UnimplementedPluginControllerServer
	bgCtx    context.Context
	mux      sync.Mutex
	listener net.Listener
	server   *grpc.Server

	loaderID        uuid.UUID
	grpcTarget      string
	network         string
	address         string
	shutdownTimeout time.Duration

	domainManager components.DomainManager
	domainPlugins map[uuid.UUID]*plugin[prototk.DomainMessage]

	transportManager components.TransportManager
	transportPlugins map[uuid.UUID]*plugin[prototk.TransportMessage]

	registryManager components.RegistryManager
	registryPlugins map[uuid.UUID]*plugin[prototk.RegistryMessage]

	signingModuleManager components.KeyManager
	signingModulePlugins map[uuid.UUID]*plugin[prototk.SigningModuleMessage]

	notifyPluginsUpdated chan bool
	notifySystemCommand  chan prototk.PluginLoad_SysCommand
	pluginLoaderDone     chan struct{}
	loadingProgressed    chan *prototk.PluginLoadFailed
	serverDone           chan error
}

func NewPluginManager(bgCtx context.Context,
	grpcTarget string, // default is a UDS path, can use tcp:127.0.0.1:12345 strings too (or tcp4:/tcp6:)
	loaderID uuid.UUID,
	conf *pldconf.PluginManagerConfig) components.PluginManager {

	pc := &pluginManager{
		bgCtx: bgCtx,

		grpcTarget:      grpcTarget,
		loaderID:        loaderID,
		shutdownTimeout: confutil.DurationMin(conf.GRPC.ShutdownTimeout, 0, *pldconf.DefaultGRPCConfig.ShutdownTimeout),

		domainPlugins:        make(map[uuid.UUID]*plugin[prototk.DomainMessage]),
		transportPlugins:     make(map[uuid.UUID]*plugin[prototk.TransportMessage]),
		registryPlugins:      make(map[uuid.UUID]*plugin[prototk.RegistryMessage]),
		signingModulePlugins: make(map[uuid.UUID]*plugin[prototk.SigningModuleMessage]),

		serverDone:           make(chan error),
		notifyPluginsUpdated: make(chan bool, 1),
		notifySystemCommand:  make(chan prototk.PluginLoad_SysCommand, 1),
		loadingProgressed:    make(chan *prototk.PluginLoadFailed, 1),
	}
	return pc
}

func (pm *pluginManager) parseGRPCAddress(ctx context.Context, serverAddr string) error {

	// We support a subset of the Go network prefixes, and if none is found, we default to UNIX Domain Sockets ("unix:")
	tcpStr, isTCP := strings.CutPrefix(serverAddr, "tcp:")
	if isTCP {
		pm.network = "tcp"
		pm.address = tcpStr
		return nil
	}
	tcp4Str, isTCP4 := strings.CutPrefix(serverAddr, "tcp4:")
	if isTCP4 {
		pm.network = "tcp4"
		pm.address = tcp4Str
		return nil
	}
	tcp6Str, isTCP6 := strings.CutPrefix(serverAddr, "tcp6:")
	if isTCP6 {
		pm.network = "tcp6"
		pm.address = tcp6Str
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
	pm.network = "unix"
	pm.address = udsPath
	return nil

}

func (pm *pluginManager) PreInit(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	return &components.ManagerInitResult{}, nil
}

func (pm *pluginManager) PostInit(c components.AllComponents) error {
	pm.domainManager = c.DomainManager()
	pm.transportManager = c.TransportManager()
	pm.registryManager = c.RegistryManager()
	pm.signingModuleManager = c.KeyManager()

	if err := pm.ReloadPluginList(); err != nil {
		return err
	}

	if err := pm.parseGRPCAddress(pm.bgCtx, pm.grpcTarget); err != nil {
		return err
	}
	return nil
}

func (pm *pluginManager) Start() (err error) {
	ctx := pm.bgCtx
	log.L(ctx).Infof("server starting on %s:%s", pm.network, pm.address)
	pm.listener, err = net.Listen(pm.network, pm.address)
	if err != nil {
		log.L(ctx).Error("failed to listen: ", err)
		return err
	}
	pm.server = grpc.NewServer()

	prototk.RegisterPluginControllerServer(pm.server, pm)

	log.L(ctx).Infof("server listening at %v", pm.listener.Addr())
	go pm.runServer(ctx)
	return nil
}

func (pm *pluginManager) runServer(ctx context.Context) {
	log.L(ctx).Infof("Run GRPC Server")

	log.L(ctx).Infof("Server started")
	pm.serverDone <- pm.server.Serve(pm.listener)
	log.L(ctx).Infof("Server ended")
}

func (pm *pluginManager) Stop() {
	ctx := pm.bgCtx
	log.L(ctx).Infof("Stop")

	gracefullyStopped := make(chan struct{})
	go func() {
		defer close(gracefullyStopped)
		pm.server.GracefulStop()
	}()
	select {
	case <-gracefullyStopped:
	case <-time.After(pm.shutdownTimeout):
		pm.server.Stop()
	}
	serverErr := <-pm.serverDone
	log.L(ctx).Infof("Server stopped (err=%v)", serverErr)

}

// Must be started to call this function
func (pm *pluginManager) GRPCTargetURL() string {
	switch pm.network {
	case "unix":
		return "unix:" + pm.listener.Addr().String()
	default:
		return "dns:///" + pm.listener.Addr().String()
	}
}

func (pm *pluginManager) LoaderID() uuid.UUID {
	return pm.loaderID
}

func (pm *pluginManager) ReloadPluginList() (err error) {
	for name, smp := range pm.signingModuleManager.ConfiguredSigningModules() {
		if err == nil {
			err = initPlugin(pm.bgCtx, pm, pm.signingModulePlugins, name, prototk.PluginInfo_SIGNING_MODULE, smp)
		}
	}
	for name, dp := range pm.domainManager.ConfiguredDomains() {
		if err == nil {
			err = initPlugin(pm.bgCtx, pm, pm.domainPlugins, name, prototk.PluginInfo_DOMAIN, dp)
		}
	}
	for name, tp := range pm.transportManager.ConfiguredTransports() {
		if err == nil {
			err = initPlugin(pm.bgCtx, pm, pm.transportPlugins, name, prototk.PluginInfo_TRANSPORT, tp)
		}
	}
	for name, tp := range pm.registryManager.ConfiguredRegistries() {
		if err == nil {
			err = initPlugin(pm.bgCtx, pm, pm.registryPlugins, name, prototk.PluginInfo_REGISTRY, tp)
		}
	}

	if err != nil {
		return err
	}

	select {
	case pm.notifyPluginsUpdated <- true:
	default:
	}
	return nil
}

func (pm *pluginManager) WaitForInit(ctx context.Context, pluginType prototk.PluginInfo_PluginType) error {
	for {
		switch pluginType {
		case prototk.PluginInfo_DOMAIN:
			unloadedPlugins, _ := unloadedPlugins(pm, pm.domainPlugins, pluginType, false)
			unloadedCount := len(unloadedPlugins)
			if unloadedCount == 0 {
				return nil
			}
		case prototk.PluginInfo_REGISTRY:
			unloadedPlugins, _ := unloadedPlugins(pm, pm.registryPlugins, pluginType, false)
			unloadedCount := len(unloadedPlugins)
			if unloadedCount == 0 {
				return nil
			}
		case prototk.PluginInfo_SIGNING_MODULE:
			unloadedPlugins, _ := unloadedPlugins(pm, pm.signingModulePlugins, pluginType, false)
			unloadedCount := len(unloadedPlugins)
			if unloadedCount == 0 {
				return nil
			}
		case prototk.PluginInfo_TRANSPORT:
			unloadedPlugins, _ := unloadedPlugins(pm, pm.transportPlugins, pluginType, false)
			unloadedCount := len(unloadedPlugins)
			if unloadedCount == 0 {
				return nil
			}
		}

		select {
		case loadErrOrNil := <-pm.loadingProgressed:
			if loadErrOrNil != nil {
				return i18n.NewError(ctx, msgs.MsgPluginLoadFailed, loadErrOrNil.ErrorMessage)
			}
		case <-ctx.Done():
			log.L(ctx).Warnf("server exiting before plugin initialization complete")
			return i18n.NewError(ctx, msgs.MsgContextCanceled)
		}
	}
}

func (pm *pluginManager) newReqContext() context.Context {
	return log.WithLogField(pm.bgCtx, "plugin_reqid", pldtypes.ShortID())
}

func (pm *pluginManager) InitLoader(req *prototk.PluginLoaderInit, stream prototk.PluginController_InitLoaderServer) error {
	ctx := pm.newReqContext()
	suppliedID, err := uuid.Parse(req.Id)
	if err != nil || suppliedID != pm.loaderID {
		return i18n.WrapError(ctx, err, msgs.MsgPluginLoaderUUIDError)
	}
	pm.mux.Lock()
	if pm.pluginLoaderDone != nil {
		pm.mux.Unlock()
		return i18n.WrapError(ctx, err, msgs.MsgPluginLoaderAlreadyInit)
	}
	pm.pluginLoaderDone = make(chan struct{})
	pm.mux.Unlock()
	log.L(ctx).Infof("Plugin loader connected")
	return pm.sendPluginsToLoader(stream)
}

func (pm *pluginManager) SendSystemCommandToLoader(cmd prototk.PluginLoad_SysCommand) {
	select {
	case pm.notifySystemCommand <- cmd:
	default:
		log.L(pm.bgCtx).Warnf("Unable to send system command to loader (command already queued)")
	}
}

func (pm *pluginManager) LoadFailed(ctx context.Context, req *prototk.PluginLoadFailed) (*prototk.EmptyResponse, error) {
	log.L(ctx).Errorf("Plugin load %s (type=%s) failed: %s", req.Plugin.Name, req.Plugin.PluginType, req.ErrorMessage)
	select {
	case pm.loadingProgressed <- req:
	default:
	}
	return &prototk.EmptyResponse{}, nil
}

func initPlugin[CB any](ctx context.Context, pm *pluginManager, pluginMap map[uuid.UUID]*plugin[CB], name string, pType prototk.PluginInfo_PluginType, conf *pldconf.PluginConfig) (err error) {
	pm.mux.Lock()
	defer pm.mux.Unlock()
	plugin := &plugin[CB]{pc: pm, id: uuid.New(), name: name}
	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
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
	pluginType, err := pldtypes.LibraryType(conf.Type).Enum().Validate()
	if err == nil {
		plugin.def.LibType, err = MapLibraryTypeToProto(pluginType.Enum())
		pluginMap[plugin.id] = plugin
	}
	return err
}

func (pm *pluginManager) tapLoadingProgressed() {
	select {
	case pm.loadingProgressed <- nil:
	default:
	}
}

func unloadedPlugins[CB any](pm *pluginManager, pluginMap map[uuid.UUID]*plugin[CB], pbType prototk.PluginInfo_PluginType, setInitializing bool) (unloaded, notInitializing []*plugin[CB]) {
	pm.mux.Lock()
	defer pm.mux.Unlock()
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
		log.L(pm.bgCtx).Debugf("%d of %d %s plugins loaded", len(pluginMap)-len(unloaded), len(pluginMap), pbType)
	} else {
		log.L(pm.bgCtx).Infof("All %s plugins loaded %v", pbType, pluginList)
	}
	return unloaded, notInitializing
}

func getPluginByIDString[CB any](pm *pluginManager, pluginMap map[uuid.UUID]*plugin[CB], idStr string, pbType prototk.PluginInfo_PluginType) (p *plugin[CB], err error) {
	pm.mux.Lock()
	defer pm.mux.Unlock()
	id, err := uuid.Parse(idStr)
	if err == nil {
		p = pluginMap[id]
		if p == nil {
			err = i18n.NewError(pm.bgCtx, msgs.MsgPluginUUIDNotFound, pbType, id)
		}
	}
	return p, err
}

func (pm *pluginManager) sendPluginsToLoader(stream prototk.PluginController_InitLoaderServer) (err error) {
	defer func() {
		pm.mux.Lock()
		defer pm.mux.Unlock()
		close(pm.pluginLoaderDone)
		pm.pluginLoaderDone = nil
	}()
	ctx := stream.Context()
	for {
		// We send a load request for each plugin that isn't new - which should result in that plugin being loaded
		// and resulting in a ConnectDomain bi-directional stream being set up.
		_, notInitializingSigningModules := unloadedPlugins(pm, pm.signingModulePlugins, prototk.PluginInfo_SIGNING_MODULE, true)
		for _, plugin := range notInitializingSigningModules {
			if err == nil {
				err = stream.Send(plugin.def)
			}
		}
		_, notInitializingDomains := unloadedPlugins(pm, pm.domainPlugins, prototk.PluginInfo_DOMAIN, true)
		for _, plugin := range notInitializingDomains {
			if err == nil {
				err = stream.Send(plugin.def)
			}
		}
		_, notInitializingTransports := unloadedPlugins(pm, pm.transportPlugins, prototk.PluginInfo_TRANSPORT, true)
		for _, plugin := range notInitializingTransports {
			if err == nil {
				err = stream.Send(plugin.def)
			}
		}
		_, notInitializingRegistries := unloadedPlugins(pm, pm.registryPlugins, prototk.PluginInfo_REGISTRY, true)
		for _, plugin := range notInitializingRegistries {
			if err == nil {
				err = stream.Send(plugin.def)
			}
		}
		if err == nil {
			select {
			case <-ctx.Done():
				log.L(ctx).Debugf("loader stream closed")
				err = i18n.NewError(ctx, msgs.MsgContextCanceled)
			case systemCommand := <-pm.notifySystemCommand:
				_ = stream.Send(&prototk.PluginLoad{SysCommand: &systemCommand})
			case <-pm.notifyPluginsUpdated:
				// loop and load any that need loading
			}
		}
		if err != nil {
			return err
		}
	}
}
