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

package commsbus

import (
	"fmt"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	talaria "github.com/kaleido-io/talaria/pkg/talaria"
)

/*
	Right - I'm not really sure what the comms bus is right now or what it looks like. It
	might be its own discrete component, or it might be the output of the transaction engine,
	I'm not sure and I don't think it matters at this rev other than knowing that there is
	something which is producing events.

	This module is mostly being used as a means to push data through the transport flow and
	show that that the rest of Talaria runtime is working as expected.

	We're cutting some corners here:

	 - The Comms bus exposes an API /message that when hit with a JSON payload, calls through
	   to talaria and starts the message send flow.
	 - I'd expect that whatever the comms bus is, is initialised with a reference to the
	   Talaria runtime, we're using that concept here but it won't look like this when we
		 actually initialise talaria 
*/


type CommsBusAPIServer struct {
	port int

	// In theory this is initalised somewhere near the top of the Paladin flow
	transportProvider talaria.TransportProvider
}

type message struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

func (cs *CommsBusAPIServer) sendMessage(w http.ResponseWriter, r *http.Request) {
	// We we get a message through, use the client lib to send it through to transport
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Unable to read request body", http.StatusBadRequest)
		return
	}

	message := &message{}
	err = json.Unmarshal(body, message)
	if err != nil {
		http.Error(w, "Unable to unmarshal request body", http.StatusBadRequest)
		return
	}

	// This is the important bit we (the comms bus) are sending messages through to Talaria to be
	// sent to another Paladin node that we know about. The content of the message here is a string
	// we don't care that inside of Talaria it's actually going to be encoded into Protobufs and
	// sent onwards.
	//
	// TODO: Change from using strings here and show something more interesting like objects.
	// ---------------------------------------------------------------------------------------------------------------------------------------
	err = cs.transportProvider.SendMessage(context.Background(), message.To, []byte(message.Content))
	if err != nil {
		http.Error(w, "Unable to unmarshal request body", http.StatusInternalServerError)
		return
	}
	// ---------------------------------------------------------------------------------------------------------------------------------------
} 

func (cs *CommsBusAPIServer) StartServer() {
	http.HandleFunc("/message", cs.sendMessage)

	err := http.ListenAndServe(fmt.Sprintf(":%d", cs.port), nil)
	if err != nil {
		log.Fatalf("Error starting server: %s", err)
	}
}

func NewCommsBusAPIServer(port int, transportProvider talaria.TransportProvider) *CommsBusAPIServer {
	return &CommsBusAPIServer{
		port: port,
		transportProvider: transportProvider,
	}
}