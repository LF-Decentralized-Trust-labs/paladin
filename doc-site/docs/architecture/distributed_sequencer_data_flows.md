# Data flows between nodes

Paladin nodes communicate data between themselves in order to build the correct on-chain transaction for a private Paladin transaction.

The basic rules for distribution of data between nodes are as follows:

1. If a node does not participate in a private transaction in any way - for example, if it is not required to endorse or assemble a transaction - Paladin does not send any data relating to the transaction to the node
2. If a node does participate in a private transaction - for example, to coordinate it or to assemble it - Paladin sends the private transaction data to those nodes
3. If a node is participating in a private transaction but is not coordinating the transaction (i.e. it is not the node who will submit the public base-ledger transaction) the coordinator will send copies of the public transaction submissions to the originator's node.
   - This is to allow the original sender of the transaction the ability to query information about public transactions relating to its private Paladin transaction

The following diagram gives an example of the data flows between nodes for a private transaction submitted byte **member1** to **Node 1**.

 - **Node 2** is the coordinating nodes
 - **Node 3** and **Node 4** do participate in the same domain, but are not involved in this particular Paladin transaction


![Data flows](diagrams/paladin-node-data-flows.svg){.zoomable-image}

Any queries to nodes 3 and 4 about the transaction to not return any information. The nodes have not received private or public transaction data.

Queries to node 1 return both private and public transaction data. However, data about the transaction is only returned to the originating identity of the transaction. If another identity on node 1 (e.g. member2 in the diagram above) requests data about TX1, the node does not return information about the transaction to it.