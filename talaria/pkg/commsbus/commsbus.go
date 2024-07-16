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

	This module is mostly being used as a means to push data through the transaction flow

*/


// OK, so we're going to cut a few corners here, we allow for messages to
// be sent through to the CommsBus as an API and then we send it through
// the transport flow
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

	// THIS IS THE IMPORTANT CODE!
	// ---------------------------------------------------------------------------------------------------------------------------------------
	err = cs.transportProvider.SendMessage(context.Background(), message.To, message.Content)
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