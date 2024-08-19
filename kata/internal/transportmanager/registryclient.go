package transportmanager

/*

	Super fake registry to mimic what the actual registry should look like once it's in place. We know that
	the JSON structure in the registry for each identity is as follows:

	{
		"name": string,
		"owner": string,
		"properties": {
			"key": "value"
			"transportInformation": {
				"grpc": "<serialised information>",
				"jms": "<serialised information>"
			}
		}
	}

	We also know that it's going to be a gRPC interface to speak to the Registry plugin, so we mock that too

*/

import (
	"sync"
)

type RegistryClient interface {
	ResolveIdentity(identity *ResolveIdentityRequest) *ResolveIdentityResponse
	GetTransportInformation(identity *ResolveIdentityResponse) map[string]string
}

type ResolveIdentityRequest struct {
	Name string
}

const TRANSPORT_INFORMATION = "transportInformation"

type ResolveIdentityResponse struct {
	Name       string
	Owner      string
	Properties map[string]interface{}
}

type registryClient struct {
	identities    map[string]*ResolveIdentityResponse
	identitiesMut sync.Mutex
}

func NewRegistryClient() *registryClient {
	return &registryClient{
		identities: make(map[string]*ResolveIdentityResponse),
	}
}

func (rc *registryClient) RegisterIdentity(name string, details *ResolveIdentityResponse) {
	rc.identitiesMut.Lock()
	defer rc.identitiesMut.Unlock()
	rc.identities[name] = details
}

func (rc *registryClient) ResolveIdentity(identity *ResolveIdentityRequest) *ResolveIdentityResponse {
	rc.identitiesMut.Lock()
	defer rc.identitiesMut.Unlock()
	return rc.identities[identity.Name]
}

func (rc *registryClient) GetTransportInformation(identity *ResolveIdentityResponse) map[string]string {
	return identity.Properties[TRANSPORT_INFORMATION].(map[string]string)
}
