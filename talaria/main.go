package main

import (
	"context"
	"flag"
	"sync"

	commsbus "github.com/kaleido-io/talaria/pkg/commsbus"
	talaria "github.com/kaleido-io/talaria/pkg/talaria"
)

/*

	Okay, some explanation to what is going on here:

	Each instance of this script stands up an interface and talaria(*1) representing a single
	paladin node. Therefore, by standing up 2 instances of this script you can demonstrate a
	comms flow between 2 paladin nodes. Each paladin node here is constructed with the arch:

	Comms Bus -> talaria -> gRPC plugin -> ... -> gRPC plugin -> talaria -> Comms Bus

	(*1) - Literally called talaria because on the diagram it looks like (0-0)
*/

var (
	commsbusport = flag.Int("commsbusport", 8080, "the port to run the comms bus on")
	registryport = flag.Int("registryport", 8081, "the port to run the registry on")
	talariaport  = flag.Int("talariaport", 8082, "the port for talaria to be listening to")
)

func main() {
	ctx := context.Background()
	flag.Parse()
	var wg sync.WaitGroup

	// Initialise the registry
	re := talaria.NewLocalAPIRegistryProvider(*registryport)

	// Initialise talaria
	b := talaria.Newtalaria(re, *talariaport)
	b.InitialisePlugins(ctx)

	// Start the comms bus
	cas := commsbus.NewCommsBusAPIServer(*commsbusport, b)
	wg.Add(1)
	go func(){
		cas.StartServer()
	}()

	wg.Wait()
}