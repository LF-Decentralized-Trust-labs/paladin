package talaria

import (
	"fmt"
	"net/http"
	"log"
	"io"
	"encoding/json"
)

/*
	Don't know what the registry looks like at the moment so let's take a big ol' stab in the dark
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
	log.Println("Adding new peer to the store...")

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
		log.Fatalf("Error starting the registry API: %s", err)
	}
}

func (larp *LocalAPIRegistryProvider) LookupPaladinEntity(identity string) (RegistryEntry, error){
	re, ok := larp.peers[identity]
	if !ok {
		return RegistryEntry{}, fmt.Errorf("could not find the named peer")
	}

	return re, nil
}
