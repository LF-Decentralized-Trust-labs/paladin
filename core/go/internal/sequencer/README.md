# Distributed Sequencer Protocol

See architecture document for the detailed specification for the distributed sequencer protocol.  This folder contains a golang implementation of that protocol.

The core component is the `ContractSequencerAgent`.
This is a non thread safe state machine that implements the distributed sequencer spec.  
Thread safety is provided by the fact that the `ContractSequencerAgent` is never called directly but instead via the `EventLoop`.  The event loop receives  messages from the transport agent and and triggers event handlers on `ContractSequencerAgent` on a single thread. The `EventLoop` also includes a heartbeat monitor and will trigger event handlers if no heartbeat notification messages are received in the expected interval.

The state of the `ContractSequencerAgent` can be safely read externally via the thread safe `ContractSequencerAgentState` interface.

//TBD - how should the `ContractSequencerAgent` event handlers update the `ContractSequencerAgentState` ? via a channel and a separate go routine? Or directly via functions protected with read/write mutexes? Or both (i.e. only one go routine would ever attempt to get the write lock and therefore no risk of deadlock).