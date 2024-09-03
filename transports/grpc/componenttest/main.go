package main

/*

	In this test we're going to fake up enough of Kata that we can get a transport manager working with a
	real compiled GRPC plugin and get it talking to a fake Paladin node (which will be a locally stood up
	GRPC server).

*/

import (
	"context"
	"net"
	"os"
	"plugin"

	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/transportmgr"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	grpctransport "github.com/kaleido-io/paladin/kata/internal/plugins/grpctransport/plugin"
)

type fakePluginController struct {
	prototk.UnimplementedPluginControllerServer

	recvMessages chan *prototk.TransportMessage
	sendMessages chan *prototk.TransportMessage
}

func (fpc *fakePluginController) ConnectTransport(stream grpc.BidiStreamingServer[prototk.TransportMessage, prototk.TransportMessage]) error {
	ctx := stream.Context()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-fpc.sendMessages:
				err := stream.Send(msg)
				if err != nil {
					return
				}
			}
		}
	}()

	for {
		inboundMessage, err := stream.Recv()
		if err != nil {
			return err
		}

		fpc.recvMessages <- inboundMessage
	}
}

func newFakePluginController(location string) (*fakePluginController, error) {
	listener, err := net.Listen("unix", location)
	if err != nil {
		return nil, err
	}

	fpc := &fakePluginController{
		recvMessages: make(chan *prototk.TransportMessage),
		sendMessages: make(chan *prototk.TransportMessage),
	}

	s := grpc.NewServer()
	prototk.RegisterPluginControllerServer(s, fpc)

	go func() {
		err := s.Serve(listener)
		if err != nil {
			return
		}
	}()

	return fpc, nil
}

func tempUDS() string {
	// Not safe to use t.TempDir() as it generates too long paths including the test name
	f, _ := os.CreateTemp("", "ut_*.sock")
	_ = f.Close()
	allocatedUDSName := f.Name()
	os.Remove(allocatedUDSName)
	return allocatedUDSName
}

type transportManagerTester struct {
	recvMessages chan components.TransportMessage
}

func newTransportManagerTester() *transportManagerTester {
	return &transportManagerTester{
		recvMessages: make(chan components.TransportMessage, 1),
	}
}

func main() {
	ctx := context.Background()
	tmTester := newTransportManagerTester()

	location := tempUDS()

	// -------------------------------------------------------------------------------------- Load up the fake plugin controller

	_, err := newFakePluginController(location)

	// -------------------------------------------------------------------------------------- Load client certs

	// realPaladinCACert, _ := os.ReadFile("../../../test/ca1/ca.crt")
	realPaladinServerCertBytes, _ := os.ReadFile("../../../test/ca1/clients/client1.crt")
	realPaladinServerCert := string(realPaladinServerCertBytes)
	realPaladinServerKeyBytes, _ := os.ReadFile("../../../test/ca1/clients/client1.key")
	realPaladinServerKey := string(realPaladinServerKeyBytes)
	realPaladinClientCertBytes, _ := os.ReadFile("../../../test/ca1/clients/client1.crt")
	realPaladinClientCert := string(realPaladinClientCertBytes)
	realPaladinClientKeyBytes, _ := os.ReadFile("../../../test/ca1/clients/client1.key")
	realPaladinClientKey := string(realPaladinClientKeyBytes)
	// fakePaladinCACert, _ := os.ReadFile("../../../test/ca2/ca.crt")
	// fakePaladinServerCert, _ := tls.LoadX509KeyPair("../../../test/ca2/clients/client1.crt", "../../../test/ca2/clients/client1.key")

	// -------------------------------------------------------------------------------------- Try and load the plugin from the .so file

	plugin, err := plugin.Open("../../../grpctransport.so")
	if err != nil {
		log.L(ctx).Errorf("error opening the shared object file %v", err)
		return
	}

	// Handle for starting the plugin
	go func() {
		runSymbol, err := plugin.Lookup("Run")
		if err != nil {
			log.L(ctx).Errorf("run function does not exist, %v", err)
			return
		}

		run, ok := runSymbol.(func(string, string))
		if !ok {
			log.L(ctx).Errorf("run function does not have the expected signature, %v", err)
			return
		}

		run("grpc", location)
	}()

	// -------------------------------------------------------------------------------------- Initialize the transport manager

	config := grpctransport.UnprocessedGRPCConfig{
		ServerCertificate: &realPaladinServerCert,
		ServerKey:         &realPaladinServerKey,
		ClientCertificate: &realPaladinClientCert,
		ClientKey:         &realPaladinClientKey,
		ExternalPort:      8081,
	}
	marshalledConfig, err := yaml.Marshal(config)

	var configNode *yaml.Node
	yaml.Unmarshal(marshalledConfig, &configNode)

	mgrConfig := transportmgr.TransportManagerConfig{
		Transports: make(map[string]*transportmgr.TransportConfig),
	}
	mgrConfig.Transports["grpc"] = &transportmgr.TransportConfig{
		Config: *configNode,
	}

	manager := transportmgr.NewTransportManager(ctx, &mgrConfig)

	// TODO: Change this when we plug the registry into the manager
	_, err = manager.Init(nil)
	if err != nil {
		log.L(ctx).Errorf("error initializing the transport manager %v", err)
		return
	}

	err = manager.Start()
	if err != nil {
		log.L(ctx).Errorf("error starting the transport manager %v", err)
		return
	}

	manager.RegisterReceiver(func(ctx context.Context, message components.TransportMessage) error {
		tmTester.recvMessages <- message
		return nil
	})

	// -------------------------------------------------------------------------------------- Test Everything End-to-End

	err = manager.Send(ctx, components.TransportMessage{
		MessageType: "something",
		Payload:     []byte("something"),
	}, "test")
	if err != nil {
		log.L(ctx).Errorf("error sending the message %v", err)
		return
	}

}
