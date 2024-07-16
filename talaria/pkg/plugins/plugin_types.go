package plugins

import (
	"context"
	pluginInterfaceProto "github.com/kaleido-io/talaria/pkg/talaria/proto"
	interPaladinProto "github.com/kaleido-io/talaria/pkg/plugins/proto"
)

type PluginRegistration struct {
	Name 					 string
	SocketLocation string
}

// All plugins are required to implement this interface in order to be mamaged by talaria
type TransportPlugin interface {

	// Methods specifically for plugin lifecycle
	GetRegistration() PluginRegistration
	Initialise(context.Context)
	Start(context.Context)

	// A Plugin MUST be able to do comms over a socket, and to other nodes
	PluginMessageFlow(context.Context, *pluginInterfaceProto.PaladinMessage) (*pluginInterfaceProto.PaladinMessageReceipt, error)
	InterPaladinMessageFlow(context.Context, *interPaladinProto.InterPaladinMessage) (*interPaladinProto.InterPaladinReceipt, error)
}
