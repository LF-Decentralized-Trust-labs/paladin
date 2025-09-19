# Transaction sequencing

Private transactions implemented using UTXO states require Paladin to compile a list of input and output UTXOs which represent the before and after state for the transaction.

Different types of private transaction have different UTXO behaviours.

For the Zeto domain for example, every UTXO belongs exlusively to the current owner and can only be spent by the current owner. This requires only basic transaction sequencing since no other identities can attempt to spend the same state.

Other domains, specifically those that can involve multiple parties potentially spending the same states, require more complex sequencing.

For the Pente domain for example, there may be 2 parties who wish to modify the same EVM storage variable. This is valid, but transaction 2 must "spend" the state that was output when transaction 1 modified the variable if both transactions are to succeed. If they both attempt to spend the same state the second transaction will fail.

The component of Paladin that manages the sequencing of private transactions for all domains is called the **Distributed Sequencer**.

![Distributed Sequencer](diagrams/paladin-sequencer.svg){.zoomable-image}

The term "distributed" refers to the fact that for multi-party domains, only one Paladin node can be sequencing transactions at a given time. The sequencing of transactions is distributed among the Paladin nodes to share the workload and resources across the peers and to ensure that if one node has a failure another node can take over transaction sequencing.
