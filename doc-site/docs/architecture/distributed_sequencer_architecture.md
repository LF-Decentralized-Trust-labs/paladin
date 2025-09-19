# Architecture

Paladin is made up of a number of discrete components, each of which handles different responsibilities within the runtime.

The distributed sequencer is one of those standalone components and the sequencer manager handles the lifetime of sequencers for different domain contracts.

The following diagram provides detail on the interfaces between other Paladin components and the sequencer. The _SequencerLifecycle_ interface provides the mechanism by which the sequencer for a specific domain contract is retrieved. If it is
the first time that sequencer has been loaded in the node, the sequencer will be instatiated before being returned. If the sequencer is already loaded it will simple be returned.

![Distributed Sequencer Architecture](diagrams/paladin-code-architecture.svg){.zoomable-image}

Depending on the type of domain contract being coordinated, Paladin messages may need to flow to and from other Paladin nodes in order to assemble, endorse, and confirm transactions. For domain contracts that are coordinated on the same node the transaction originated from, no communication with other nodes is required.

The sequencer architecture uses internal events to progress transactions through the sequencer state machine. If communication with other nodes is node required, events are flowed directly back into the state machine rather than being transmitted across the network.

For more information about the different sequencer state machines and how they work see the [state machine](./distributed_sequencer_state_machine.md) topic.
