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

package talaria

import (
	"fmt"
	"net/http"
	"io"
	"encoding/json"
)

/*
	Similar to the comms bus, we're not really sure what the registry looks like, but it's definitely not
	going to look like this. Right now, we're defining an interface for what we think interactions to the
	registry look like and then implementing a basic HTTP API behind it which allow us to poke through to
	the messaging flow.

	TODO: Refine this with whatever our interface to the registry looks like (maybe more gRPC)?
*/

type RegistryEntry struct {
	SupportedPlugins   map[string]struct{} `json:"supportedPlugins"`
	RoutingInformation string              `json:"routingInformation"`
	TransactingEntity  string              `json:"transactingEntity"`
}

type RegistryProvider interface {
	LookupPaladinEntity(identity string) (RegistryEntry, error)
}

type LocalAPIRegistryProvider struct {
	port  int
	peers map[string]RegistryEntry
}

func NewLocalAPIRegistryProvider(port int) *LocalAPIRegistryProvider {
	larp := &LocalAPIRegistryProvider{
		port: port,
		peers: make(map[string]RegistryEntry),
	}

	go func(){
		larp.listenAndServe()
	}()

	return larp
}

func (larp *LocalAPIRegistryProvider) addPeer(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Unable to read request body", http.StatusBadRequest)
		return
	}

	registryEntry := &RegistryEntry{}
	err = json.Unmarshal(body, registryEntry)
	if err != nil {
		http.Error(w, "Unable to unmarshal request body", http.StatusBadRequest)
		return
	}

	larp.peers[registryEntry.TransactingEntity] = *registryEntry
}

func (larp *LocalAPIRegistryProvider) listenAndServe() {
	http.HandleFunc("/peer", larp.addPeer)

	err := http.ListenAndServe(fmt.Sprintf(":%d", larp.port), nil)
	if err != nil {
		return
	}
}

func (larp *LocalAPIRegistryProvider) LookupPaladinEntity(identity string) (RegistryEntry, error){
	re, ok := larp.peers[identity]
	if !ok {
		return RegistryEntry{}, fmt.Errorf("could not find the named peer")
	}

	return re, nil
}
