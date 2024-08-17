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
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	pbp "github.com/kaleido-io/paladin/kata/pkg/proto/plugins"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	pb "google.golang.org/protobuf/proto"
)

type PluginController interface {
	Run(ctx context.Context) error
	Stop(ctx context.Context)
	SocketAddress() string
	LoaderID() uuid.UUID
	WaitForInit(ctx context.Context) error
	PluginsUpdated(conf *PluginControllerConfig) error
}

// Runtime items to be supplied by process, including the config
type PluginControllerArgs struct {
	LoaderID      uuid.UUID
	SocketAddress string
	DomainManager DomainManager
	InitialConfig *PluginControllerConfig
}

type plugin[CB any] struct {
	pc   *pluginController
	name string
	id   uuid.UUID
	def  *pbp.PluginLoad

	initializing bool
	registered   bool
	initialized  bool
	handler      *domainHandler
}

type pluginController struct {
	pbp.UnimplementedPluginControllerServer
	bgCtx    context.Context
	mux      sync.Mutex
	listener net.Listener
	server   *grpc.Server

	loaderID        uuid.UUID
	socketAddress   string
	domainManager   DomainManager
	shutdownTimeout time.Duration

	domainPlugins  map[uuid.UUID]*plugin[DomainCallbacks]
	domainRequests *inflightManager[*pbp.DomainMessage]

	notifyPluginsUpdated chan bool
	pluginLoaderDone     chan struct{}
	loadingProgressed    chan *pbp.PluginLoadFailed
	serverDone           chan error
}

func NewPluginController(bgCtx context.Context, args *PluginControllerArgs) (_ PluginController, err error) {

	pc := &pluginController{
		bgCtx: bgCtx,

		loaderID:        args.LoaderID,
		socketAddress:   args.SocketAddress,
		shutdownTimeout: confutil.DurationMin(args.InitialConfig.GRPC.ShutdownTimeout, 0, *DefaultGRPCConfig.ShutdownTimeout),

		domainManager:  args.DomainManager,
		domainPlugins:  make(map[uuid.UUID]*plugin[DomainCallbacks]),
		domainRequests: newInFlightRequests[*pbp.DomainMessage](),

		serverDone:           make(chan error),
		notifyPluginsUpdated: make(chan bool, 1),
		loadingProgressed:    make(chan *pbp.PluginLoadFailed, 1),
	}

	if err := pc.PluginsUpdated(args.InitialConfig); err != nil {
		return nil, err
	}

	log.L(bgCtx).Infof("server starting at unix socket %s", pc.socketAddress)
	pc.listener, err = net.Listen("unix", pc.socketAddress)
	if err != nil {
		log.L(bgCtx).Error("failed to listen: ", err)
		return nil, err
	}
	pc.server = grpc.NewServer()

	pbp.RegisterPluginControllerServer(pc.server, pc)

	log.L(bgCtx).Infof("server listening at %v", pc.listener.Addr())
	return pc, nil
}

func (pc *pluginController) Run(ctx context.Context) error {
	log.L(ctx).Infof("Run GRPC Server")

	log.L(ctx).Infof("Server started")
	pc.serverDone <- pc.server.Serve(pc.listener)
	log.L(ctx).Infof("Server ended")
	return nil
}

func (pc *pluginController) Stop(ctx context.Context) {
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

func (pc *pluginController) SocketAddress() string {
	return pc.socketAddress
}

func (pc *pluginController) LoaderID() uuid.UUID {
	return pc.loaderID
}

func (pc *pluginController) PluginsUpdated(conf *PluginControllerConfig) error {
	for name, dp := range conf.Domains {
		if err := initPlugin(pc.bgCtx, pc, pc.domainPlugins, name, pbp.PluginInfo_DOMAIN, dp); err != nil {
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
		unloadedDomainPlugins, _ := unloadedPlugins(pc, pc.domainPlugins, pbp.PluginInfo_DOMAIN, false)
		unloadedCount := len(unloadedDomainPlugins)
		if unloadedCount == 0 {
			return nil
		}
		select {
		case loadErrOrNil := <-pc.loadingProgressed:
			if loadErrOrNil != nil {
				return errors.Errorf(loadErrOrNil.ErrorMessage)
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

func (pc *pluginController) InitLoader(req *pbp.PluginLoaderInit, stream pbp.PluginController_InitLoaderServer) error {
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

func (pc *pluginController) LoadFailed(ctx context.Context, req *pbp.PluginLoadFailed) (*pbp.EmptyResponse, error) {
	log.L(ctx).Errorf("Plugin load %s (type=%s) failed: %s", req.Plugin.Name, req.Plugin.PluginType, req.ErrorMessage)
	select {
	case pc.loadingProgressed <- req:
	default:
	}
	return &pbp.EmptyResponse{}, nil
}

func (pc *pluginController) ConnectDomain(stream pbp.PluginController_ConnectDomainServer) error {
	return pc.domainServer(stream)
}

func initPlugin[CB any](ctx context.Context, pc *pluginController, pluginMap map[uuid.UUID]*plugin[CB], name string, pType pbp.PluginInfo_PluginType, conf *PluginConfig) (err error) {
	pc.mux.Lock()
	defer pc.mux.Unlock()
	plugin := &plugin[CB]{pc: pc, id: uuid.New(), name: name}
	if err := types.Validate64SafeCharsStartEndAlphaNum(ctx, name, "name"); err != nil {
		return err
	}
	plugin.def = &pbp.PluginLoad{
		Plugin: &pbp.PluginInfo{
			Id:         plugin.id.String(),
			Name:       name,
			PluginType: pType,
		},
		Location: conf.Location,
	}
	plugin.def.LibType, err = types.MapEnum(conf.Type, golangToProtoLibTypeMap)
	pluginMap[plugin.id] = plugin
	return err
}

func (p *plugin[CB]) notifyInitialized() {
	p.pc.mux.Lock()
	p.initialized = true
	p.pc.mux.Unlock()
	log.L(p.pc.bgCtx).Errorf("Plugin load %s (type=%s) completed", p.def.Plugin, p.def.Plugin.PluginType)
	p.pc.tapLoadingProgressed()
}

func (pc *pluginController) tapLoadingProgressed() {
	select {
	case pc.loadingProgressed <- nil:
	default:
	}
}

func unloadedPlugins[CB any](pc *pluginController, pluginMap map[uuid.UUID]*plugin[CB], pbType pbp.PluginInfo_PluginType, setInitializing bool) (unloaded, notInitializing []*plugin[CB]) {
	pc.mux.Lock()
	defer pc.mux.Unlock()
	for _, plugin := range pluginMap {
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
		log.L(pc.bgCtx).Infof("All plugins loaded")
	}
	return unloaded, notInitializing
}

func getPluginByIDString[CB any](pc *pluginController, pluginMap map[uuid.UUID]*plugin[CB], idStr string, pbType pbp.PluginInfo_PluginType) (p *plugin[CB], err error) {
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

func (pc *pluginController) sendPluginsToLoader(stream pbp.PluginController_InitLoaderServer) (err error) {
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
		_, notInitializing := unloadedPlugins(pc, pc.domainPlugins, pbp.PluginInfo_DOMAIN, true)
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

func jsonProto(msg pb.Message) string {
	b, _ := protojson.Marshal(msg)
	return string(b)
}
