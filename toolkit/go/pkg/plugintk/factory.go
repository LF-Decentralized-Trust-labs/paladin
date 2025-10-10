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
package plugintk

import (
	"context"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
)

type PluginBase interface {
	// Run on the base is called by the Run() of the specific implementation.
	// Blocks the caller until stopped.
	Run(grpcTarget, pluginID string)
	Stop()
}

type PluginConnector[M any] func(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[M, M], error)

type pluginFactory[M any] struct {
	mux        sync.Mutex
	pluginType prototk.PluginInfo_PluginType
	instances  map[string]*pluginInstance[M]
	connector  PluginConnector[M]
	impl       PluginImplementation[M]
}

func NewPluginBase[M any](
	pluginType prototk.PluginInfo_PluginType,
	connector PluginConnector[M],
	impl PluginImplementation[M],
) PluginBase {
	return &pluginFactory[M]{
		instances:  make(map[string]*pluginInstance[M]),
		pluginType: pluginType,
		connector:  connector,
		impl:       impl,
	}
}

func (pf *pluginFactory[M]) instanceStarted(inst *pluginInstance[M]) {
	pf.mux.Lock()
	defer pf.mux.Unlock()
	if _, existing := pf.instances[inst.id]; existing {
		// Considered exceptional case - the plugin loader should not have done this
		panic("duplicate load " + inst.id)
	}
	pf.instances[inst.id] = inst
}

func (pf *pluginFactory[M]) instanceStopped(inst *pluginInstance[M]) {
	log.L(context.Background()).Infof("%s plugin instance starting %s", pf.pluginType, inst.id)
	pf.mux.Lock()
	defer pf.mux.Unlock()
	delete(pf.instances, inst.id)
}

func (pf *pluginFactory[M]) instanceList() []*pluginInstance[M] {
	pf.mux.Lock()
	defer pf.mux.Unlock()
	list := make([]*pluginInstance[M], 0, len(pf.instances))
	for _, inst := range pf.instances {
		list = append(list, inst)
	}
	return list
}

func (pf *pluginFactory[M]) Run(connString, pluginID string) {
	log.L(context.Background()).Infof("%s plugin factory starting", pf.pluginType)
	inst := newPluginInstance(pf, connString, pluginID)
	pf.instanceStarted(inst)
	defer pf.instanceStopped(inst)
	inst.run()
}

func (pf *pluginFactory[M]) Stop() {
	log.L(context.Background()).Infof("%s plugin factory stopping", pf.pluginType)
	instances := pf.instanceList()
	for _, inst := range instances {
		inst.cancelCtx()
	}
	for _, inst := range instances {
		<-inst.done
	}
}
