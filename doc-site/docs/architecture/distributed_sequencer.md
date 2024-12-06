# Distributed Sequencer Protocol

In domains (such as Pente) where the spending rules for states allow any one of a group of parties to spend the state, then we need to coordinate the assembly of transactions across multiple nodes so that we can maximize the throughput by speculative spending new states and avoid transactions being reverted due to double concurrent spending / state contention.

To achieve this, it is important that we have an algorithm that allows all nodes to agree on which of them should be selected as the coordinator at any given point in time. And all other nodes delegate their transactions to the coordinator.

## Objectives 

The objective of this algorithm is to maximize efficiency ( reduce probably for revert leading to retry cycles of valid request) and throughput ( allow many transactions to be included in each block).  This algorithm does not attempt to provide a guarantee on final data consistency but instead relies on the base ledger contract to do so (e.g. double spend protection, attestation validation, exactly once intent fulfillment).

 The desired properties of that algorithm are

 - **deterministic**: all nodes can run the algorithm and eventually agree on a single coordinator at any given point in time. This is a significant property because it means we don't want to rely on a message-based leader election process like in some algorithms such as Raft. This reduces the overhead of message exchanges among the nodes
 - **fair**: the algorithm results in each node being selected as coordinator for a proportional number of times over a long enough time frame
 - **fault tolerant**:  Although pente already depends on all nodes being available (because of the 100% endorsement model) the desired algorithm should be future proof and be compatible with <100% endorsement model where network faults and down time of a minority of nodes can bee tolerated.

## Summary
The basic premises of the algorithm are:

 - composition of committee i.e. the set of nodes who are candidates for coordinator is universally agreed (similar to BFT algorithms).
 - liveness of the coordinator node can be detected via heartbeat messages (similar to RAFT).
 - ranking of the preference for coordinator selection for any given contract address, for any given point in time ( block height) is a deterministic function that all nodes will agree on given the same awareness of make up of committee
 - choice of coordinator can change over time either due increase in block height triggering a rotation of the responsibility or the current coordinator being detected as unavailable. 
 - situations can arise where different nodes chose different coordinators because of different awareness of block height and/or different awareness of availability.  The algorithm is less efficient when this happens but continues to function and can return to full efficiency as soon as the situation is resolved.
 - there is no need for election `term`s in this algorithm.
 - when coordinator responsibility is switched to another node, each inflight transaction is either re-assigned to the new coordinator or flushed through to confirmation on the base ledger
 - the sender node for each transaction is responsible for ensuring that the transaction always has one coordinator actively coordinating it by detecting and responding to situations where the current coordinator becomes available, a more preferred coordinator comes back online or preference changes due to new block height.  (unless the sender deems the transaction to be no longer valid in which case, it marks it as reverted).
 - The sender node continues to monitor and control the delegation of its transaction until it has received receipt of the transactions' confirmations on the base ledger. This provides an "at least once" quality of service for every transaction.
 - The handshake between the sender node and the coordinator node(s) attempts to minimize the likelihood of the same transaction intent resulting in 2 valid base ledger transactions but cannot eliminate that possibility completely so there is protection against duplicate intent fulfillment in the base ledger contract

## Detailed Walkthrough
To describe the algorithm in detail, we break it down to sub problems

### Composition of committee
When creating a new Pente domain contract instance ( i.e. privacy group) the list of nodes that are candidates for coordinator selection is declared.  For Pente this happens to be exactly equal to the privacy group but potentially could theoretically be a subset of the privacy group.

Similarly, any other future domain that opts in to the dynamic coordinator selection policy,  must provide this information as part of the constructor handshake between the domain and the core paladin engine.

### Coordinator selection

This is a form of leader election but is achieved through deterministic calculation for a given point in time ( block height), given predefined members of the group, and a common (majority) awareness of the current availability of each member. So there is no actual "election" term needed.

Each node is allocated positions on a hash ring.  These positions are a function of the node name.  Given that the entire committee agrees on the names of all nodes, all nodes independently arrive at the same decision about the positions of all nodes.

For the current block height, a position on the ring is calculated from a deterministic function.  The node that is closest to that position on the ring is selected as the coordinator.  If that node is not available, then the next closest node is selected. If that node is not available, then the next closest again is selected, and so on.

For each Pente domain instance ( smart contract API) the coordinator is selected by choosing one of the nodes in the configured privacy group of that contract

 - the selector is a pure function where the inputs are the node names + the current block number and the output is the name of one of the nodes
 - the function will return the same output for a range of `n` consecutive blocks ( where `n` is a configuration parameter of the policy)
 - for the next range of `n` blocks, the function will return a different ( or possibly the same) output
 - over a large sample of ranges, the function will have an even distribution of outputs
 - the function will be implemented by taking a hash of each node name, taking the hash of the output of `b/n` (where b is the block number and `n` is the range size) rounded down to the nearest integer and feeding those into a hashring function.

![image](../images/distributed_sequencer_hashring.svg){ width="500" }

In the example here, lets say we chose a hash function with a numeric output between 0 and 360 where the hash of node A's name happens to be 315, node B is 45, node C is 225 and node D is 135.  Lets say we have a range size of 4, therefore for block numbers 0-3, we can calculate that they belong to range number 0, blocks 4-7 belong to range number 1 and so on.  Lets say that the hash function, when applied to `range0` returns 310.  When applied to `range1` returns 55 and when applied to `range2` returns 120.  Interpreting all of those hash output as degrees and plotting them on the circle ( with 0° at the top). We can then deterministically calculate that nodeA is selected while the block height is between 0 and 3, node B is selected when the block height is between 4 and 7 and node D is selected while the block height is between 8 an 11


### Node availability detection

When a node starts acting as the coordinator, then it  periodically sends a heartbeat message to all nodes that for which it has an active delegation and will accept delegation requests from any other node in the group. 

All sender nodes keep track of the current selected coordinator ( either by reevaluating the deterministic function periodically or caching the result until block height exceeds the current range boundary).  If they fail to receive a heartbeat message from the current coordinator, then they will chose the next closest node in the hashring (e.g. by reevaluating the hashring function with the unavailable nodes removed).

For illustration, consider the following example where nodes B, C, and D have recognized node A as the coordinator and have delegated transactions (labelled `B-1`, `C-1` and `D-1` )to it.

![image](../images/distributed_sequencer_availability_frame1.svg){ width="500" }


Node A then proceeds to broadcast heartbeat messages

![image](../images/distributed_sequencer_availability_frame2.svg){ width="500" }


Suddenly, Node A stops sending heartbeat messages, either because it has crashed or it has lost network connectivity to other nodes

![image](../images/distributed_sequencer_availability_frame3.svg){ width="500" }


When nodes B and D realize that the heartbeats have stopped, then they consider node A as unavailable and each independently select node C as the next closest node on the ring and delegate their inflight transactions to it.  Similarly, nodeC selects itself as the coordinator and continues to coordinate its own transaction `C-1` along with the delegated transactions from node B and node D.

![image](../images/distributed_sequencer_availability_frame4.svg){ width="500" }


Node C then proceeds to act as coordinator and sends periodic heartbeat messages to the other nodes. 

![image](../images/distributed_sequencer_availability_frame5.svg){ width="500" }


If Node A comes back online, then it will begin transmitting heartbeat messages again because it has selected itself as coordinator given the current block height.

![image](../images/distributed_sequencer_availability_frame6.svg){ width="500" }


As soon as nodes B. C and D receive the heartbeat messages from node A they each independently concur that node A is the preferred choice for coordinator and delegate any inflight transactions to it.  Node C decides to cease acting as coordinator and abandons any transactions that it has in flight.

![image](../images/distributed_sequencer_availability_frame1.svg){ width="500" }


 This universal conclusion is reached without any communication necessary between nodes B, C and D and without any knowledge of recent history.  Therefore is tolerant to any failover of nodes B, C or D in the interim and has a deterministic conclusion, that can be predicted by node A regardless of what other activity happened to occur while node A was offline (e.g. if node C had gone offline and coordinator role switched to B, or switch back to C ) 

### Network partition
The sender node ultimately bears the responsibility of selecting a coordinator for their transactions. If a single coordinator is universally chosen, the algorithm operates accordingly with maximum efficiency and data integrity (transactions are successfully confirmed on base ledger and no state is double spent). 

If multiple nodes select different coordinators, the system’s efficiency suffers significantly but the eventual "correctness" of the on chain state is not adversely affected. The algorithm is designed to minimize the likelihood of this occurrence. However, the probability of this eventuality is never zero. For instance, in the case of network partitions, this exact situation can emerge.  In this case the base ledger contract provides protection against double spending and all transactions may even eventually be processed (depending on how the endorsement policy is affected) albeit at a much reduced efficiency given the likely hood for failure and retry.

Given the behavior of the current version of Pente domain, when there is a network partition situation then neither coordinator will be able to achieve endorsement due to the 100% endorsement policy.  However, as a more general design and analysis, we consider the hypothetical future case of <100% endorsement.  For certain endorsement policies, each coordinator could theoretically get to the point of submitting transactions to the base ledger.   Each coordinator is operating with a different domain context ( a different awareness of which states have been spent / are earmarked to be spent)  therefore transactions from different coordinators would attempt to spend the same state.  Depending on the relative order that those transactions are added to a block, then transactions from one side or the other of the partition will be reverted and will need to be reassembled.

Whether the coordinators were able to limp along or had came to a halt, or some combination, in either case, once the network connectivity has been restored, the system returns to an efficient operational state.

In the following illustration, a network partition occurs which resulting in nodes A and B continuing to communicate with each other but not with nodes C or D meanwhile nodes C and D can continue to communicate with each other.

![image](../images/distributed_sequencer_partition_frame1.svg){ width="500" }


Once the network connectivity has been restored, the heartbeat messages from both coordinators are received by all other nodes.

![image](../images/distributed_sequencer_partition_frame2.svg){ width="500" }

At this point, node B and node C independently conclude that node A is the preferred coordinator and accordingly, they delegate their transactions to node A.  Node C ceases to act as coordinator and abandons any transactions that it has in flight.

![image](../images/distributed_sequencer_partition_frame3.svg){ width="500" }

The system reaches the desired eventual state of node A being coordinator, sending heartbeat messages to all nodes

![image](../images/distributed_sequencer_partition_frame4.svg){ width="500" }

TODO

 - might be useful to illustrate the "limp mode" in more detail where both coordinators are managing to get transactions through but the base ledger contract is providing double spend protection and the retry strategy of the coordinator means that transactions are eventually processed correctly 
 - all of the above seems to assume that the network partition affects communication between paladin nodes but not the blockchain.  Should really discuss and elaborate what happens when the block chain nodes are not able to communicate with each other ( esp. in case of public chains where long finality times are possible).  The TL;DR is - notwithstanding this algorithm, the paladin core engine and its management of domain contexts must be mindful of the difference between block receipts and finality and should have mechanisms to configure the number of confirmed blocks it considers as final and/or to reset domain contexts and redo transaction intents that ended up on blocks that got reversed.
 
### Coordinator switchover
When the block height reaches a range boundary then all nodes independently reach a universal conclusion the new coordinator.

Lets consider the case where the scenario explored above continues to the point where the blockchain moves to block 4.  In the interim, while node A was coordinator, nodes B, C and D delegated transactions `B-1`, `B-2`, `B-3`,  `C-1`, `C-2`, `C-3`, `D-1`, `D-2` and `D-3`,  and A itself has assembled `A-1`, `A-2`, `A-3`.

`A-1` , `B-1`, `C-1` and `D-1` have already been submitted to base ledger and confirmed.  
`A-2` , `B-2`, `C-2` and `D-2` have been prepared for submission, assigned a signing address and a nonce but have not yet been mined into a confirmed block.  NOTE it is irrelevant whether these transactions have been the subject of an `eth_sendTransaction` yet or not.  i.e. whether they only exist in the memory of the paladin node's transaction manager or whether they exist in the mempool of node A's blockchain node.  What is important is that

 - they have been persisted to node A's database as being "dispatched" and
 - are not included in block 3 or earlier

![image](../images/distributed_sequencer_switchover_frame1.svg){ width="500" }

As all nodes in the group ( including node A) receive receipts of block 3, they will recognize node B as the new coordinator


![image](../images/distributed_sequencer_switchover_frame2.svg){ width="500" }

Node A will continue to process and monitor receipts for transactions `A-2` , `B-2`, `C-2` and `D-2` but will abandon transactions `A-3` , `B-3`, `C-3` and `D-3`.

![image](../images/distributed_sequencer_switchover_frame3.svg){ width="500" }

Node A sends a `DelegationReturned` message to the respective sender nodes for each non dispatched transaction that it had in flight.


![image](../images/distributed_sequencer_switchover_frame4.svg){ width="500" }

The final transmission from node A, as coordinator, is a message to node B with details of the speculative domain context predicting confirmation of transactions `A-2` , `B-2`, `C-2` and `D-2` .  This includes the state IDs for any new states created by those transactions and any states that are spent by those transaction.  This message also includes the transaction hash for the final transaction that is submitted on the sequence coordinated by node A.  Node B needs to receive this message so that it can proceed with maximum efficiency. Therefore this message is sent with an assured delivery quality of service (similar to state distribution).  

Meanwhile, all nodes delegate transactions  `A-3` , `B-3`, `C-3` and `D-3` to node B.  

![image](../images/distributed_sequencer_switchover_frame5.svg){ width="500" }

Node B will start to coordinate the assembly and endorsement of these transactions but hold off from preparing them for dispatch to its transaction manager yet.

![image](../images/distributed_sequencer_switchover_frame6.svg){ width="500" }

Once  transactions `A-2` , `B-2`, `C-2` and `D-2` have been confirmed in a block ( which may be block 4 or may take more than one block) and node B has received confirmation of this, then node B will continue to prepare and submit transactions `A-3` , `B-3`, `C-3` and `D-3`.

![image](../images/distributed_sequencer_switchover_frame7.svg){ width="500" }

Eventually transactions `A-3` , `B-3`, `C-3` and `D-3` are mined to a block.

![image](../images/distributed_sequencer_switchover_frame8.svg){ width="500" }

Note that there is a brief dip in throughput as node B waits for node A to flush through the pending dispatched transactions and there is also some additional processing for the inflight transactions that haven't been dispatched yet.  So a range size of 4 is unreasonable and it would be more likely for range sizes to be much larger so that these dips in throughput become negligible.

TODO

 - need more detail on precisely _how_  nodes B, C and D know that transactions `B-2``A-2` , `B-2`, `C-2` and `D-2` are past the point of no return and that transactions `A-3` , `B-3`, `C-3` and `D-3` do need to be re-delegated.
 - need more detail on precisely _how_ node B can continue to coordinate the assembly of transactions `A-3` , `B-3`, `C-3` and `D-3` in a domain context that is aware of the speculative states locks from transactions `B-2``A-2` , `B-2`, `C-2` and `D-2` 
 - It might be more productive for the next level of detail on these points to come in the form of a proposal in code.

### Variation in block height
It is likely that different nodes will become aware of new block heights at different times so the algorithm must accommodate that inconsistency. 

 - given that different nodes index the blockchain events with varying latency, it is not assumed that all nodes have the same awareness of "current block number" at any one time. This is accommodated by the following
 - Each node delegates all transactions to which ever node it determines as the current coordinator based on its latest knowledge of "current block"
 - The delegate node will accept the delegation if its awareness of current block also results in it being chosen by the selector function.  Otherwise, the delegate node rejects the delegation and includes its view of current block number in the response
 - On receiving the delegation rejection, the sender node can determine if it is ahead or behind (in terms of block indexing) the node it had chosen as delegate.  
 - If the sender node is ahead, it continues to retry the delegation until the delegate node finally catches up and accepts the delegation
 - If the sender node is behind, it waits until its block indexer catches up and then selects the coordinator for the new range
 - Coordinator node will continue to coordinate ( send endorsement requests and submit endorsed transactions to base ledger) until its block indexer has reached a block number that causes the coordinator selector to select a different node.
 - at that time, it waits until all dispatched transactions are confirmed on chain, then delegates all current inflight transactions to the new coordinator.
 - if the new coordinator is not up to date with block indexing, then it will reject and the delegation will be retried until it catches up. 
 - while a node is the current selected coordinator, it sends endorsement requests to every other node for every transaction that it is coordinating
 -  The endorsement request includes the name of the coordinator node
 - Each endorsing node runs the selector function to determine if it believes that is the correct coordinator for the current block number
 - if not, then it rejects the endorsement and includes its view of the current block number in the rejection message
 - when the coordinator receives the rejection message, it can determine if it is ahead or behind the requested endorser
 - if the coordinator is ahead, it retries the endorsement request until the endorser catches up and eventually endorses the transaction
 - if the coordinator is behind, then it waits until its block indexer reaches the next range boundary and delegates all inflight transactions to the new coordinator



### Sender's responsibility

The sender node for any given transaction remains ultimately responsible for ensuring that transaction is successfully confirmed on chain or finalized as failed if it is not possible to complete the processing for any reason.  While the coordination of assembly and endorsement is delegated to another node, the sender continues to monitor the progress and is responsible for initiating retries or re-delegation to other coordinator nodes as appropriate.

Feedback available to the sender node that can be monitored to track the progress or otherwise of the transaction submission:

 - when the sender node is choosing the coordinator, it may have recently received a heartbeat message from the preferred coordinator or an alternative coordinator
 - when sending the delegation request to the coordinator, the sender node expects to receive an acknowledgement that the request has been received.  This is not a guarantee that the transaction will be completed.  At this point, the coordinator has only an in-memory record of that delegated transaction
 - coordinator heartbeat messages.  The payload of these messages contains a list of transaction IDs that the coordinator is actively coordinating
 - transaction confirmation request.  Once the coordinator has fulfilled the attestation plan, it sends a message to the transaction sender requesting permission to dispatch. If, for any reason, the sender has already re-delegated to another coordinator, then it will reject this request otherwise, it will accept.
 - transaction dispatched message.  When the coordinator prepares a transaction for submission to base ledger, it sends a message to the sender node for that transaction to communicate that fact.  The delivery of the message happens after the coordinator has written to its local database. The same database transaction triggers the downstream process to submit the transaction to the blockchain node.  The submission process happens concurrently to the transaction dispatched message.  So when the `sender` node receives the `transaction dispatched message`, there is no guarantee whether the transaction has been submitted to base ledger yet or not.  However, there is an assurance that the coordinator node has persistent the intent to submit it and will endeavor to do so even if it fails over.  However there are error situations where this is not possible (e.g. it could lose access to the signing key, or lose connectivity to the blockchain node) and these situations may be transient or may be permanent. 
 - blockchain event.  The base ledger contract for each domain emits a custom event that included the transaction ID.  The sender node will detect when such an event corresponds to one of its transactions and will record that transaction as confirmed.
 - transaction reverted message.  When the submitter fails to submit the transaction, for any reason, and gives up trying , then a `TransactionReverted` message is sent to the sender of that transaction.  There are some cases where the submitter retries certain failures and does *not* send this message.

Decisions and actions that need to be taken by the sender node

 - When a user sends a transaction intent (`ptx_sendTransaction` or `ptx_prepareTransaction`), the sender node needs to chose which coordinator to delegate to.
 - If the block height changes and there is a new preferred coordinator as per the selection algorithm then the sender node needs to decide whether to delegate the transaction to it. This will be dependent on whether the transaction has been dispatched or not.
 - If the coordinator node seems to have forgotten about the transaction, then the sender node needs to decide to re-delegate it
 - If the preferred coordinator node becomes unavailable then the sender node needs to decide which alternative coordinator to delegate to
 - If a transaction has been delegated to an alternative coordinator and the preferred coordinator becomes available again, then the sender needs to decide to re-delegate to the preferred coordinator
 - Given that there could have been a number of attempts to delegate and re-delegate the transaction to different coordinators, when any coordinator reaches the point of dispatching the transaction, the sender needs to decide whether or not it is valid to dispatch at the time, by that coordinator
 - If the base ledger transaction is reverted for any reason, then the sender decides to retry

To illustrate, lets consider the following scenarios

 - simple happy path: sender delegates transaction to a coordinator and that coordinator eventually successfully submits it to the base ledger contract
 - happy path with switchover: sender delegates transaction to a coordinator but before the transaction is ready for submission, the block height changes, a new coordinator is selected which then  eventually successfully submits it to the base ledger contract
 - realistic happy path: given that different nodes are likely to become aware of block height changes at different times, there are 6 different permutations of the order in which the 3 nodes ( sender, previous coordinator, new coordinator) become aware of the new block height but to understand the algorithm, we can reduce the analysis to 3 more general cases: When the sender is behind, when the previous coordinator is behind and when the new coordinator is behind.
 - failover cases: there is no persistence checkpoints between the transaction intent being received by the sender node and the transaction being dispatched to the base ledger submitter so if either the sender or coordinator node process restarts, then it loses all context of that delegation
     - sender failover.  If the sender restarts, it reads all pending transactions from its database and delegates each transaction to the chosen coordinator for that point in time. This may be the same coordinator that it delegates to previously or may be a new one. The sender holds, in memory, the name of the current coordinator and will only accept a dispatch confirmation from that coordinator.  So if it happens to be the case that it had delegated to a different coordinator before it restarted, then it will reject the dispatch confirmation from that coordinator and that copy of the assembled transaction will be discarded. Noting that this would trigger a re-assembly and re-run of the endorsement flow for any other transactions, including those from other senders, that have been assembled to spend the states produced by the discarded transaction. However, in most likeliness, all other senders will switch their preference to the same new coordinator and trigger a re-do of all non dispatched transactions.
     - coordinator failover. When the coordinator restarts, it will continue to send heartbeat messages but those messages only contain the transaction ids for the transactions that were delegated to it since restart.  For any pending transactions that were previously delegated to it, when the sender of those transactions receives the heartbeat message and realizes the that coordinator has "forgotten" about the transaction, then the sender re-delegates those transactions.
 - coordinator becomes unavailable:  If the sender stops receiving heartbeat messages from the coordinator, then it assumes that the coordinator has become unavailable.  However it may actually be the case that the coordinator is still running and just that the network between the coordinator and the sender has a fault.  The action of the sender for each inflight transaction depends on exactly which stage of the flow the sender believes that transaction to be.  The consequence of the sender's action depend on whether the previous coordinator is still operating and where in the flow it believes the transaction to be.  Scenarios to explore are:
     -  sender detects that the heartbeats have stopped before it has sent a response to a dispatch confirmation request (or before it has received the dispatch confirmation request)
     -  sender detects that the heartbeats have stopped after it has received  
  

#### Simple happy path

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  actor user
  participant S as Sender
  participant C as Coordinator
  participant E as Endorser
  user->>S: ptx_sendTransaction
  activate S
  S -->> user: TxID
  deactivate S
  S->>C: delegate
  par endorsement flow
    C->>S: Assemble
    S-->>C: Assemble response
    loop for each endorser
      C -) E: endorse
      E -) C: endorsement
    end
  and heartbeats
    loop every heartbeat cycle
      C -) S: heartbeat
    end
  end
  Note over S, C: once all endorsements <br/>have been gathered
  C->>S: request dispatch confirmation
  S->>C: dispatch confirmation
  create participant SB as Submitter
  C->>SB: dispatch
  C->>S: dispatched
  
```

#### Coordinator switchover
```mermaid

sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  actor user
  participant S as Sender
  participant C1 as First <br/> Coordinator
  participant C2 as New <br/> Coordinator
  participant E as Endorser
  box rgba(33,66,99,0.5) blockchain 
    participant BC1 as First Coordinators <br/> blockchain node
    participant BC2 as New Coordinators <br/> blockchain node
    participant BS as Senders <br/>blockchain node
  end

  user->>S: ptx_sendTransaction
  activate S
  S -->> user: TxID
  deactivate S
  S->>C1: delegate
  par endorsement flow
    C1->>S: Assemble
    S-->>C1: Assemble response
    loop for each endorser
      C1 -) E: endorse
      E -) C1: endorsement
    end
  and heartbeats
    loop every heartbeat cycle
      C1 -) S: heartbeat
    end
  end
  Note over S, C1: Before transaction is ready for <br/> dispatch, the block height changes
  BC1 -) C1: new block
  BC2 -) C2: new block
  BS -) S: new block
  
  S->>C2: delegate
  par endorsement flow
    C2->>S: Assemble
    S-->>C2: Assemble response
    loop for each endorser
      C2 -) E: endorse
      E -) C2: endorsement
    end
  and heartbeats
    loop every heartbeat cycle
      C2 -) S: heartbeat
    end
  end
  C2->>S: request dispatch confirmation
  S->>C2: dispatch confirmation
  create participant SB as Submitter
  C2->>SB: dispatch
  C2->>S: dispatched
  
```

#### Coordinator switchover where the new coordinator is behind on block indexing
```mermaid

sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  actor user
  participant S as Sender
  participant C1 as First <br/> Coordinator
  participant C2 as New <br/> Coordinator
  participant E as Endorser
  box rgba(33,66,99,0.5) blockchain 
    participant BC1 as First Coordinators <br/> blockchain node
    participant BC2 as New Coordinators <br/> blockchain node
    participant BS as Senders <br/>blockchain node
  end

  note over S, E: Transaction delegated but not yet dispatched
  BS -) S: new block
  BC1 -) C1: new block

  loop until delegate accepted
    S->>C2: delegate
    C2-->>S: delegate rejected
    Note over S, C2: rejection message includes New Coordinators <br/> current block height
  end

  BC2 -) C2: new block

  S->>C2: delegate
  C2-->>S: delegate accepted

  note over S, E: Endorsement flow completes as per happy path

  C2->>S: request dispatch confirmation
  S->>C2: dispatch confirmation
  create participant SB as Submitter
  C2->>SB: dispatch
  C2->>S: dispatched
  
```


#### Coordinator switchover where original coordinator is behind on block indexing
```mermaid

sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  actor user
  participant S as Sender
  participant C1 as First <br/> Coordinator
  participant C2 as New <br/> Coordinator
  participant E as Endorser
  box rgba(33,66,99,0.5) blockchain 
    participant BC1 as First Coordinators <br/> blockchain node
    participant BC2 as New Coordinators <br/> blockchain node
    participant BS as Senders <br/>blockchain node
  end

  note over S, E: Transaction delegated but not yet dispatched
  BS -) S: new block
  BC2 -) C2: new block
  
  S->>C2: delegate
  C2-->>S: delegate accepted

  par 
    note over S, E: Endorsement flow completes as per happy path
    C2->>S: request dispatch confirmation
    S->>C2: dispatch confirmation
    create participant SB as Submitter
    C2->>SB: dispatch
    C2->>S: dispatched
  and
    BC1 -) C1: new block 
    C1->>S: request dispatch confirmation
    S->>C1: dispatch confirmation rejected
  end
  
  
```

#### Coordinator switchover where Sender is behind on block indexing
```mermaid

sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  actor user
  participant S as Sender
  participant C1 as First <br/> Coordinator
  participant C2 as New <br/> Coordinator
  participant E as Endorser
  box rgba(33,66,99,0.5) blockchain 
    participant BC1 as First Coordinators <br/> blockchain node
    participant BC2 as New Coordinators <br/> blockchain node
    participant BS as Senders <br/>blockchain node
  end

  BC1 -) C1: new block
  BC2 -) C2: new block
  Note over S, C2: both coordinators are already aware that the <br/> block height has changed but the sender still <br/> believes that the first coordinator is the preferred one 
  
  user->>S: ptx_sendTransaction
  activate S
  S -->> user: TxID
  deactivate S
  S->>C1: delegate
  Note over S, C2: delegation message contains the sender's block height <br/> and because it is in a different block range <br/> from the selected coordinator,<br/> the delegation is rejected with an explicit reason which includes the coordinator's block height.
  C1-->>S: delegation rejection
  
  Note over S: Sender waits until it catches up with <br/>the block range.  It could still be behind <br/> in block height but waits until it is <br/>in the correct range.

  BS -) S: new block

  S->>C2: delegate
  C2-->>S: delegation accepted
  Note over S, C2: flow continues as per the happy path
  
```

Theoretically, the sender could trust the response from the first coordinator and delegate to the new coordinator even though the sender itself has not witnessed the blockchain get to that height.  This may be a point of discussion for a future optimization.

#### Failover

Before illustrating the failover scenarios, we shall add some detail to the happy path relating to the persistence points.  Previous diagrams omitted this detail because all activity is controlled by the in-memory view of the state machine and persistence only becomes relevant when we consider the cases where we loose one or more node's in-memory state.

This diagram is annotated 🔥 with interesting points where a failover could occur

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  actor user
  participant SDB as Sender's Database
  participant S as Sender
  participant C as Coordinator
  participant CDB as Coordinator's Database
  participant E as Endorser
  user->>S: ptx_sendTransaction
  activate S
  S -->> user: TxID
  S ->> SDB: Insert transaction
  deactivate S
  S->>C: delegate
  par endorsement flow
    C->>S: Assemble
    S-->>C: Assemble response
    loop for each endorser
      C -) E: endorse
      E -) C: endorsement
    end
  and heartbeats
    loop every heartbeat cycle
      C -) S: heartbeat
    end
  end
  Note over S, C: once all endorsements <br/>have been gathered 
  %Reliable handshake to request final permission to dispatch
  C->>S: request dispatch confirmation
  activate S
  S ->> SDB: Insert dispatch confirmation
  activate SDB
  Note over S: 🔥1S  
  S ->> SDB: Commit
  deactivate SDB
  deactivate S
  Note over S: 🔥2S

  %Reliable handshake to grant final permission to dispatch
  S->>C: dispatch confirmation
  activate C
  C->>CDB: Insert dispatch
  activate CDB
  Note over C: 🔥1C  
  C->>CDB: Commit
  deactivate CDB
  deactivate C
  Note over C: 🔥2C 

  %Actually do the dispatch
  create participant SB as Submitter
  C->>SB: dispatch
  
  %Reliable handshake to deliver the notification for dispatch
  C->>S: dispatchedNotification
  activate S
  S ->> SDB: Insert dispatch
  activate SDB
  Note over S: 🔥2S
  S ->> SDB: Commit
  deactivate SDB
  deactivate S
  Note over S: 🔥3S

  %acknowledge the notification of dispatch
  S->>C: dispatchNotificationAcknowledgement
  activate C
  C->>CDB: Insert dispatchNotificationAcknowledgement
  activate CDB
  Note over C: 🔥2C  
  C->>CDB: Commit
  deactivate CDB
  deactivate C
  Note over C: 🔥3C 

  
```

#### Sender failover

In the case where there is no change in coordinator selection after the failover, this scenario is a simple case of the sender re-loading its pending transactions from persistence and sending a delegation request to the coordinator which is received in an idempotent manner.  The complexities here depends on exactly when the sender restarts.  i.e. whether it is 
 - on or before 🔥1S 
 - on or between either of the 🔥2S markers 
 - on or after point 🔥3S
  
```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  actor user
  participant SDB as Sender's Database
  participant S as Sender
  participant C as Coordinator
  participant E as Endorser
  user->>S: ptx_sendTransaction
  activate S
  S -->> user: TxID
  S ->> SDB: Insert transaction
  deactivate S
  S->>C: delegate

  par endorsement flow
    C->>S: Assemble
    S-->>C: Assemble response
    loop for each endorser
      C -) E: endorse
      E -) C: endorsement
    end
  and heartbeats
    loop every heartbeat cycle
      C -) S: heartbeat
    end
  and sender failover
   note over SDB,C : in parallel to the normal endorsement flow and heartbeat,<br/> the sender happens to restart and resends<br/> the delegation request to the coordinator
   S->>SDB: Get pending transactions
   S->>C: Delegate
   note right of C: this request matches an in-flight delegation<br/> so is received as a no-op to the coordinator
  end

 %Reliable handshake to request final permission to dispatch
  C->>S: request dispatch confirmation
  activate S
  S ->> SDB: Insert dispatch confirmation
  activate SDB
  note over SDB, C: 🔥1S if the sender restarts before committing the dispatch confirmation to its DB,<br/> then it never sends the the confirmation to the coordinator <br/>and the coordinator will retry
  S ->> SDB: Commit
  deactivate SDB
  deactivate S
  Note over S: 🔥2S if the sender restarts after committing the dispatch confirmation to its DB<br/> but before it has sent the confirmation to the coordinator,<br/> then it will send the confirmation when it restarts


%Reliable handshake to grant final permission to dispatch
  S->>C: dispatch confirmation
  activate C
  C->>CDB: Insert dispatch
  activate CDB
  C->>CDB: Commit
  deactivate CDB
  deactivate C
  %Actually do the dispatch
  create participant SB as Submitter
  C->>SB: dispatch
  
  %Reliable handshake to deliver the notification for dispatch
  C->>S: dispatchedNotification
  activate S
  S ->> SDB: Insert dispatch
  activate SDB
  S ->> SDB: Commit
  deactivate SDB
  deactivate S
  Note over S: 🔥3S If the sender restarts after this point,<br/> it has nothing else to do other than wait for <br/>the confirmation from blockchain (or revert). <br/>  If it happens that it did not get as far as sending the <br/> dispatchNotificationAcknowledgement then the <br/>coordinator will retry sending the `dispatchedNotification`

  %acknowledge the notification of dispatch
  S->>C: dispatchNotificationAcknowledgement
  activate C
  C->>CDB: Insert dispatchNotificationAcknowledgement
  activate CDB
  C->>CDB: Commit
  deactivate CDB
  deactivate C
  
```

The case where the sender selects a different coordinator after it restarts is no different to the case where the sender selects a new coordinator without a restart.  Whether that is because of a new block range or because the old coordinator is no longer available.  The fact that the sender has "forgotten" that it had previously delegate to a different coordinator does not change anything.  In the non-restart cases, the sender does not send any communications to the original coordinator ( i.e. it makes no attempt to actively claw back the delegation) - although doing so may be a subject for discussion on future optimization.  The only impact of the coordinator switch is when the original coordinator requests permission to dispatch.  It will be rejected because the sender has an in-memory record that it has delegate to the new coordinator which is exactly the same situation whether the sender had restarted before doing so or not.

#### Coordinator failover

From the perspective of the coordinator if it restarts on or before point 🔥1C, then it is as if it has never seen any delegation for that transaction.  It may or may not get a new delegation depending on whether the sender selects that same coordinator by the time it realizes that this coordinator is no longer actively coordinating that transaction.

If it restarts on or after point 🔥2C


NOTE: all of the above discussion assumes that there is a timely recovery in the case of failover.  If the coordinator fails and does not recover in a timely manner, then that is a similar scenario to the coordinator becoming unavailable.

**Assured once only confirmed transaction per intent**
The sender makes every effort to ensure that only one transaction is submitted to the base ledger for any given user request but it is impossible for the sender to guarantee that so the algorithm relies on validation on the base ledger contract that no transaction `intent` will be confirmed more than once.

TODO Need to flesh out more detail on how the base ledger achieves this.


### Complete sequence diagram

All of the above combined into a single diagram with links to the details description for each message exchange and the processing that each message triggers on the receiving node.

TBD: not sure if this should be kept but it is useful as a TODO list for writing the detail protocol points

```mermaid

sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  autonumber
  actor user
  participant S as Sender
  participant C1 as First <br/> Coordinator
  participant C2 as New <br/> Coordinator
  participant E as Endorser
  box rgba(33,66,99,0.5) blockchain 
    participant BC1 as First Coordinators <br/> blockchain node
    participant BC2 as New Coordinators <br/> blockchain node
    participant BS as Senders <br/>blockchain node
  end

  user->>S: ptx_sendTransaction
  activate S
  S -->> user: TxID
  deactivate S
  S->>C1: delegateCommand

  par endorsement flow
    C1->>S: Assemble
    S-->>C1: Assemble response
    loop for each endorser
      C1 -) E: endorse
      E -) C1: endorsement
    end
  and heartbeats
    loop every heartbeat cycle
      C1 -) S: heartbeat
    end
  end
  Note over S, C1: Before transaction is ready for <br/> dispatch, the block height changes
  BC1 -) C1: new block
  BC2 -) C2: new block
  BS -) S: new block
  
  S->>C2: delegate
  par endorsement flow
    C2->>S: Assemble
    S-->>C2: Assemble response
    loop for each endorser
      C2 -) E: endorse
      E -) C2: endorsement
    end
  and heartbeats
    loop every heartbeat cycle
      C2 -) S: heartbeat
    end
  end
  C2->>S: request dispatch confirmation
  S->>C2: dispatch confirmation
  create participant SB as Submitter
  C2->>SB: dispatch
  C2->>S: dispatched
  
```

## Protocol points

To understand the detailed specification of the responsibilities and expectations of each node in the network, we explore the detail of message formats, message exchange handshake and processing related to the handling of specific messages.  We start with an overview of some common message exchange patterns before drilling down to the detailed concrete specification of messages and their handlers.

### Common message exchange patterns
All messages contain the following fields:

  - `protocolVersion` semver version number that can be used to determine whether the software version running the sender and receiver nodes are compatible with each other.
  - `messageID` unique id for every message.

Reply messages also contain

  - `CorrelationID` this is a copy of the `MessageID` of the message that this reply corresponds to.

These can be used by the transport layer plugins to exploit qualities of service capabilities of the chosen transport. 


In addition to these, the distributed sequencer protocol defines some common message exchange patterns that provide delivery assurances and correlation in the context of the handshakes described in the protocol walkthrough above.

<!-- TBD: `CorrelationID` is not currently used in the implementation.  Should we remove this or redefine its behavior (i.e. to be a copy of the `RequestID`) or leave it as an option for the transport plugin to use? -->

#### Assured delivery

In some cases, it important for one party to have some assurance that the other party has received a message and has triggered an action based on that message.  The action may be to store the message in a persistent store or may be to trigger some in memory process.  
The approach to achieving assured delivery is to define message exchange patterns where the messages are expected to be reciprocated with some form of acknowledgment.

Messages sent with an assured delivery expectation have an additional field that is unique to the intent of the message but unlike the `MessageID` is not necessarily unique for every message.  This allows the sender to idempotenty resend the message in lieu of any acknowledgment and achieve and `at least` once assurance. Each resend would be a different `MessageID` therefore allowing the transport layer to assume that `MessageID` is unique and avoid the transport layer from discarding the retry message as a duplicate.
 
There are three patterns of exchange that provide assured delivery that we shall explore in detail below

 - `ReliableNotification`
 - `Command` / `CommandAccepted` / `CommandRejected`
 - `Request` / `Response` / `Error`

#### Non assured delivery

In some cases, assured delivery is not necessary and simple datagram message exchange pattern is sufficient.  In those cases, typically to comminute point in time status messages ( e.g. heartbeat), if a message is lost, there is no loss in data and the correct status will be communicated via the next message that does manage to reach its destination. 

#### Reliable notification 
The `Reliable notification` message exchange pattern involves 2 parties

 - notification sender
 - notification receiver

The `notification sender` sends a `Notification` message and will continue to resend periodically until it has received an `Acknowledgment` from the notification receiver.

From the receiver's perspective, a reliable notification is idempotent.  The state of the receiver must not differ when it receives multiple notifications with matching notification id.  The receiver must send an acknowledgment when it successfully receives a notification, even if it has already sent an acknowledgment for a previous notification with the same notification id.

From the sender's perspective, an acknowledgement to a reliable notification is idempotent. The state of the sender must not differ when it receives multiple acknowledgment with matching notification ids.  It must stop resending notifications with that notification id when it receives at least one acknowledgment and it must tolerate receiving further acknowledgements for that same notification id. 


```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  participant S as Sender
  participant R as Receiver
  loop Every RetryInterval
    S -) R: Notification
  end
  R --) S: Acknowledgement
```

#### Assured delivery data distribution

This pattern combines the `ReliableNotification` pattern with a persistence layer on both nodes to achieve assured eventual consistency between the data stores.

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  
  participant S as Sender
  participant SDB as Sender's Database
  participant R as Receiver
  participant RDB as Receiver's Database
  S->>SDB: InsertDistributionOperation
  activate SDB
  S->>SDB: Commit
  deactivate SDB
  loop Every RetryInterval
    S -) R: DistributeData
  end
  R->>RDB: InsertDistributedData
  activate RDB
  R->>RDB: Commit
  deactivate RDB
  
  R --) S: Acknowledgement

  S->>SDB: InsertDistributionAcknowledgement
  activate SDB
  S->>SDB: Commit
  deactivate SDB
  
```

#### Command 
The `Command` message exchange pattern involves 2 parties

 - command sender
 - command receiver

The `command sender ` sends a `ReliableMessage` containing information about the command and will continue to resend periodically until it has received either a  `CommandAccepted` or `CommandRejection` message.

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  participant S as Command Sender
  participant R as Command Receiver
  loop Every RetryInterval
    S -) R: Command
  end
  alt command valid
    R --) S: CommandAccepted
  else
    R --) S: CommandRejected
  end
```

The reliable command pattern is used to assure that the sequencer code running in the other node's memory space has received the command and does not give any assurance to the persistence of that command. Theoretically, a this pattern could be combined with database persistence to provide that assurance. However, currently there are not cases of that in the protocol.

Typically, the `Reliable command` pattern is combined with `Reliable notification` pattern to feedback to the `command sender` when the command has been completed or aborted.  But this is not always the case.

#### Request
The `Request` message exchange pattern involves 2 parties

 - request sender
 - request receiver

The `request sender ` sends a `Request` containing a query for some data continue to resend the request periodically until it has received either a  `RequestResponse` containing the requested data or a `RequestError` message containing the reason for the error.

The sender will resend the request periodically until it receives a response or an error.

From the perspective of the receiver, the request may be treated as idempotent.  If it has already sent a response to a request with the same request id, then it must send another response (or error). In this case the request receiver may sent the exact same response as previously but may alternatively send a different response.  In other words, there is no obligation on the request receiver to remember exactly what requests it has responded to and what those responses were.  It must however always send a valid response or error.  In cases where the request receiver has received  and responded to multiple copies of the same request ( matching request id), there is no guarantee which of the responses will be consumed by the request sender.

From the perspective of the request sender, only one request response should be consumed and all other should be ignored.  If an error is received and the reason for the error is a transient one, there is no guarantee that sending the same request (matching request id) in future will result in a different response.  The retry protocol is intended to mitigate unreliability at the network transport layer ( i.e. when no response is received).  To retry in the case of other transient error's, a new request (i.e. a message with a different request id) must be initiated. 

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  participant S as Request Sender
  participant R as Request Receiver
  loop Every RetryInterval
    S -) R: Request
  end
  alt request is valid
    R --) S: Response
  else
    R --) S: Error
  end
```

### Message specification and handler responsibilities

The following concrete message exchanges defines the responsibilities and expectation of each node in the network.



#### <a name="message-transaction-delegation-command"></a>Transaction delegation


This is an instance of the `Command` pattern where the `command sender` is the sender node for the given transaction and the `command receiver` is the node that has been chosen, by the `sender` as the `coordinator`.  The `sender` sends a `DelegationCommand` message...

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  participant S as Sender
  participant C as Coordinator
  loop Every RetryInterval
    S -) C: DelegationCommand
  end
  alt delegation valid
    C --) S: DelegationAccepted
    C -->> C: Coordinate Transaction
    activate C
    deactivate C
  else
    C --) S: DelegationRejected
  end
  
```


##### <a name="message-transaction-delegation-command"></a>Delegation Command
```proto
message DelegationCommand {
    string transaction_id = 1;
    string delegate_node_id = 2;
    string delegation_id = 3; //this is used to correlate the acknowledgement back to the delegation. unlike the transport message id / correlation id, this is not unique across retries
    bytes private_transaction = 4; //json serialized copy of the in-memory private transaction object
    int64 block_height = 5; // the block height upon which this delegation was calculated (the highest delegation wins when crossing in the post)
}
```

As a member of the coordinator committee, a node that receives a `DelegationCommand` must either respond by sending `DelegationAccepted` or `DelegationRejected` message.  

Valid reasons for rejection are
 - requesters block height is in a different block range than expected.  In this case, the `DelegationRejected` message must contain the block height that the local node is aware of so that the delegating node can make an informed decision based on whether they are behind or ahead.
 - the current node is not the most preferred available coordinator for the current block height.

If the Delegation is accepted, then the receiving node must begin [coordinating that transaction](#subprocess-coordinate-transaction)

```mermaid
---
title: OnDelegationCommand
---
flowchart TB
    A@{ shape: sm-circ, label: "Start" }
    B@{ shape: diam, label: "Check <br/>block<br/>range" }
    C@{ shape: lean-r, label: "Reject<br/>(wrong block height)" }
    D@{ shape: fr-rect, label: "Select<br/>available<br/>coordinator" }
    E@{ shape: diam, label: "this Is<br/>Coordinator" }
    I@{ shape: lean-r, label: "Reject<br/>(wrong coordinator)" }
    G@{ shape: fr-rect, label: "<a href="#subprocess-coordinate-transaction">coordinate transaction</a>" }
    J@{ shape: fr-circ, label: " " }


    A-->B
    B --> |Behind| C
    B --> |Ahead| C
    B --> |Same| D
    D --> E
    E --> |Yes| G
    E --> |No| I
    G --> J
```

##### <a name="message-transaction-delegation-rejected"></a>Transaction delegation rejected

The handling of a delegation rejected message depends on the reason for rejection.
 - if the reason is `MismatchedBlockHeight` and the target delegate is ahead then a new delegation request is sent once the sender has reached a compatible block height.  Compatible block height is defined as a block height in the same block range. 
 - if the reason is `MismatchedBlockHeight` and the target delegate is behind then a new delegation request is sent on a periodic interval until it is accepted or rejected with a different reason
 - if the reason is `NotPreferredCoordinator` the `SendTransaction` process is reset

```proto
message DelegationRequestRejected {
    string transaction_id = 1;
    string delegate_node_id = 2;
    string delegation_id = 3;
    string contract_address = 4;
    DelegationRequestRejectedReason reject_reason = 5;
    optional MismatchedBlockHeightDetail mismatchedBlockHeightDetail = 6; //included if reject_reason is "MismatchedBlockHeight"
    optional NotPreferredCoordinatorDetail notPreferredCoordinatorDetail = 7; //included if reject_reason is "NotPreferredCoordinator"
}

enum DelegationRequestRejectedReason{
  MismatchedBlockHeight = 0;
  NotPreferredCoordinator = 1;
}

message MismatchedBlockHeightDetail {
  int64 provided_block_height = 1;
  int64 observed_block_height = 2;
}

message NotPreferredCoordinatorDetail {
  string preferred_coordinator = 1;
  google.protobuf.Timestamp latest_heartbeat_time = 2;
  string latest_heartbeat_id = 3;
}

```

##### <a name="message-transaction-delegation-accepted"></a>Transaction delegation accepted

If a node receives a `DelegationAccepted` message then it should start to monitor the continued acceptance of that delegation.  It can expect to receive `HeartbeatNotification` messages from the delegate node and for those messages to include the id of the delegated transaction. The `sender` node cannot assume that the `coordinator` node will persist the delegation request.  If the heartbeat messages stop or if the received heartbeat messages do not contain the expected transaction ids, then the sender should retrigger the `HandleTransaction` process to cause the transaction to be re-delegated either to the same delegate, or new delegate or to be coordinated by the sender node itself. Whichever is appropriate for the current point in time.  

```proto
message DelegationRequestAccepted {
    string transaction_id = 1;
    string delegate_node_id = 2;
    string delegation_id = 3;
    string contract_address = 4;
}
```

#### Transaction Assemble


This is an instance of the `Request` pattern where the `request sender` is the coordinator node for the given transaction and the `request receiver` is the sender node.

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  participant S as Sender
  participant C as Coordinator
  loop Every RetryInterval
    C -) S: AssembleRequest
  end
  alt is valid
    S --) C: AssembleResponse
  else
    S --) C: AssembleError
  end
```  

Given that a node (referred to as `sender`) has delegated a transaction to another node (referred to as `delegate`) and hasn't since re-delegated that transaction for any reason to any other node, when the `sender` node receives an `AssembleRequest` message from the `delegate` then it should invoke the relevant domain code, with a domain context containing the state locks from the `AssembleRequest` message, sign the resulting payload and send an `AssembleResponse` message to the `delegate`.

The `sender` should send an `AssembleError` message if it is unable to successfully assemble and sign the transaction for any reason.

If no `AssembleResponse` or `AssembleError` message is received by the coordinator after the `assembleTimeout` period, then the coordinator may abandon this transaction or make another attempt to re-assemble it, at a later time, potentially with a new domain context.

The `sender` node must not assume that a successful `AssembleResponse` will be the final assemble of that transaction.  If further `AssembleRequest` messages are received then the `sender` must fulfil those in a timely manner otherwise the  `coordinator` may stop coordinating that transaction.

If the `AssembleRequest` does not correspond to an inflight transaction for that `sender` node then the sender must send a `AssembleError` message to the coordinator node.
If the `AssembleRequest` does no correspond to the active delegation for the given transaction, then the `sender` node must send an `AssembleError` to the coordinator node.


A note on "active delegation" vs "inflight transaction":  A given transaction is determined to be `in-flight` if the sender has received a `ptx_sendTransaction(s)` or `ptx_prepareTransaction(s)` call from the user and the transaction has not yet been dispatched to the submitter or prepared and distributed back to the sender.  While a transaction is in-flight, it may be the subject of multiple delegations but only one of those delegations are considered as `active` at any point in time.  For example, the sender node may detect that the coordinator has gone offline and will then create a new delegation for an alternative coordinator.


##### <a name="message-assemble-request"></a>Assemble request

```proto
message AssembleRequest {
    string transaction_id = 1;
    string assemble_request_id = 2;
    string contract_address = 3;
    bytes transaction_inputs = 4;
    bytes pre_assembly = 5;
    bytes state_locks = 6;
    int64 block_height = 7;
    string delegation_id = 8;
}
```

```mermaid
---
title: OnAssembleRequest
---
flowchart TB
    A@{ shape: sm-circ, label: "Start" }
    B@{ shape: h-cyl, label: "Inflight<br/>transactions" }
    C@{ shape: rect, label: "Query<br/>inflight<br/>transactions" }
    D@{ shape: rect, label: "Send AssembleError" }
    E@{ shape: diam, label: "Is<br/>active<br/>delegation" }
    F@{ shape: rect, label: "Send AssembleError" }
    G@{ shape: rect, label: "Assemble" }
    H@{ shape: rect, label: "Sign<br/>Payload" }
    I@{ shape: rect, label: "Send AssembleResponse" }

    A --> C
    C <--> B
    C --> |Not Found| D
    C --> E
    E --> |No| F
    E --> |Yes| G
    G --> H
    H --> I
```

##### <a name="message-assemble-response"></a>Assemble response

```proto
message AssembleResponse {
    string transaction_id = 1;
    string assemble_request_id = 2;
    string contract_address = 3;
    bytes post_assembly = 4;
}
```

##### <a name="message-assemble-error"></a>Assemble error

```proto
message AssembleError {
    string transaction_id = 1;
    string assemble_request_id = 2;
    string contract_address = 3;
    string error_message = 4;
}
```

If a coordinator receives an `AssembleError` then it must stop coordinating that transaction.  It is the responsibility of the sender node to decide whether to finalize the transaction as `failed` or to retry at a later point in time.

#### Transaction endorsement


This is an instance of the `Request` pattern where the `request sender` is the coordinator node for the given transaction and the `request receiver` is one of the required endorsers.

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  participant C as Coordinator
  participant E as Endorser
  loop Every RetryInterval
    C -) E: EndorsementRequest
  end
  alt is valid
    E --) C: EndorsementResponse
  else
    E --) C: EndorsementError
  end
```  

As a member of the endorsement committee for a domain instance, whenever a node receives an `Endorsement request` it must respond, with either an `EndorsementResponse` or `EndorsementError`.

##### <a name="message-endorsement-request"></a>Endorsement request

```proto
message EndorsementRequest {
    google.protobuf.Any attestation_request = 1;
    string idempotency_key = 2;
    string transaction_id = 3;
    string contract_address = 4;
    string party = 5;
    google.protobuf.Any transaction_specification = 6;
    repeated google.protobuf.Any verifiers = 7;
    repeated google.protobuf.Any signatures = 8;
    repeated google.protobuf.Any inputStates = 9;
    repeated google.protobuf.Any readStates = 10;
    repeated google.protobuf.Any outputStates = 11;
    repeated google.protobuf.Any infoStates = 12;
}
```

##### <a name="message-endorsement-response"></a>Endorsement response

```proto
message EndorsementResponse {
    string transaction_id = 1;
    string idempotency_key = 2;
    string contract_address = 3;
    optional google.protobuf.Any endorsement = 4;
    string party = 5;
    string attestation_request_name = 6;
}
```

##### <a name="message-endorsement-response"></a>Endorsement error

When a coordinator receives a `EndorsementError` it must remove that transaction, and any dependant transactions from the current graph of assembled transactions and queue them up for re-assembly.

```proto
message EndorsementError {
    string transaction_id = 1;
    string idempotency_key = 2;
    string contract_address = 3;
    optional string revert_reason = 4;
    string party = 5;
    string attestation_request_name = 6;
}
```

#### Heartbeat

Once a `coordinator` node has accepted a delegation, it will continue to periodically send `HeartbeatNotification` messages to the `sender` node for that transaction. The coordinator sends a heartbeat notification to all nodes for which it is currently coordinating transactions.  The message sent to any given node must include the ids for all active delegations. The message may contain other delegation ids that are unknown to the receiving node.

This is a fire and forget notification.  There is no acknowledgement expected.

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  participant C as Coordinator
  participant S1 as Sender 1
  participant S2 as Sender 2
  loop Every `HeartBeatInterval`
    C -) S1: Heartbeat
    C -) S2: Heartbeat
  end
  
```  

##### <a name="message-heartbeat-notification"></a>Heartbeat notification

When a node receives a heartbeat message from a coordinator node, it should compare that notification with its record of inflight transactions and active delegations
 - if the receiving node has any in flight transactions that have been delegated to coordinators other than the source of the received heartbeat, then the receiving node should initiate a re-evaluation of its choice of coordinator

```proto
message HeartbeatNotification {
    string heartbeat_id = 1;
    google.protobuf.Timestamp timestamp = 2;
    string idempotency_key = 3;
    string contract_address = 4;
    repeated string transaction_ids = 5;
}
```

#### Dispatch confirmation

When a coordinator has gathered all required endorsements for a transaction and there are no dependant transactions still waiting to be endorsed, then the coordinator must request confirmation of dispatch from the sender of the transaction. This step is not strictly necessary to achieve data integrity but it does help to mitigate situations where the delegation had been rescinded and a transaction delegated to another coordinator without the initial coordinator being aware of that. If this confirmation is not sought, then there is a possibility that the coordinator will cause the submitter and / or another coordinator / submitter to perform wasteful processing submitting a transaction that will revert on the base ledger as a duplicate.

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  participant C as Coordinator
  participant S as Sender
  loop Every RetryInterval
    C -) S: RequestDispatchConfirmation
  end
  alt request is valid
    S --) C: DispatchConfirmationResponse
  else
    R --) S: Error
  end
```

##### <a name="message-request-dispatch-confirmation"></a> Request dispatch confirmation

Given that a node (referred to as `sender`) has delegated a transaction to another node (referred to as `delegate`) and hasn't since re-delegated that transaction for any reason to any other node, when the `sender` node receives a `RequestDispatchConfirmation` message from the `delegate` then it must send a `DispatchConfirmationResponse` message.
If the sender does not recognize the delegation or if it has knowingly relegated to another coordinator, then it must send a `DispatchConfirmationError`

TODO need more detail here

##### <a name="message-dispatch-confirmation-response"></a> Dispatch confirmation response

```proto
message DispatchConfirmationResponse {
    
}
```

This is the successful response to a `Request dispatch confirmation` message.  

##### <a name="message-dispatch-confirmation-error"></a> Dispatch confirmation error

```proto
message HeartbeatNotification {
    string heartbeat_id = 1;
    google.protobuf.Timestamp timestamp = 2;
    string idempotency_key = 3;
    string contract_address = 4;
    repeated string transaction_ids = 5;
}
```


#### Dispatch notification

Once a coordinator has dispatched a transaction to a submitter, it must notify the sender via a dispatch notification flow which is an instance of the `Assured delivery data distribution` pattern.

```mermaid
sequenceDiagram
  %%{init: { "sequence": { "mirrorActors":false }}}%%
  
  participant C as Coordinator
  participant CDB as Coordinator's Database
  participant S as Sender
  participant SDB as Senders's Database
  C->>CDB: InsertDispatchNotificationOperation
  activate CDB
  C->>CDB: Commit
  deactivate CDB
  loop Every RetryInterval
    C -) S: 
  end
  S->>SDB: InsertDispatchNotification
  activate SDB
  S->>SDB: Commit
  deactivate SDB
  
  S --) C: DispatchNotificationAcknowledgement

  C->>CDB: InsertDispatchNotificationAcknowledgement
  activate CDB
  C->>CDB: Commit
  deactivate CDB
```


##### <a name="message-dispatch-notification"></a> Dispatch notification



##### <a name="message-dispatch-notification-acknowledgment"></a> Dispatch notification acknowledgment

  

### Sub processes

#### <a name="subprocess-handle-transaction"></a> Handle transaction

The `HandleTransaction` process is the main process implemented byt the sender node for a transaction. It is initially triggered as an effect of the user calling `ptx_sendTransaction(s)` ( or `ptx_prepareTransaction(s)`) and can be re-triggered in certain error cases or process restarts.

```mermaid
---
title: Handle transaction
---
flowchart TB
    Start@{ shape: sm-circ, label: "Start" }
    A@{ shape: fr-rect, label: "Select<br/>available<br/>coordinator" }
    B@{ shape: diam, label: "Is Local<br/>Coordinator" }
    C@{ shape: fr-rect, label: "<a href="#subprocess-coordinate-transaction">coordinate transaction</a>" }
    D@{ shape: fr-rect, label: "Delegate transaction" }
    
    Start-->A
    A --> B
    B --> |Yes| C
    B --> |No| D

```

#### <a name="subprocess-coordinate-transaction"></a> Coordinate transaction 
The `Coordinate transaction` subprocess is responsible for assembling the transaction, gathering endorsements, preparing the transaction for dispatch and then requesting confirmation from the sender before proceeding to dispatch for submission to the base ledger or distributed back to the sender for future submission.

There is no guarantee that the coordinator will complete the preparation any may prematurely stop coordinating the transaction for an unexpected reason (e.g. process restart). While the coordinator does continue to coordinate the transaction, it must include that transaction's id in the heartbeat message that it sends to the sender of that transaction.  The heartbeat message may contain transactions that were delegated by other senders but a heartbeat message sent to a given node must at least contain the transaction ids for which that node is a sender.

It is responsibility of the `sender` to send a new `DelegationRequest` to the coordinator, or to another coordinator node if there is any indication that the original coordinator has prematurely stopped coordinating a transaction for any reason.

```mermaid
---
title: Coordinate transaction
---
flowchart TB
    Start@{ shape: sm-circ, label: "Start" }
    A@{ shape: rect, label: "Wait for assemble slot" }
    B@{ shape: fr-rect, label: "Assemble Transaction" }
    C@{ shape: fr-rect, label: "Gather endorsements" }
    D@{ shape: diam, label: "Submit<br/>mode" }
    E@{ shape: fr-rect, label: "Distribute prepared transaction" }
    F@{ shape: fr-rect, label: "Confirm dispatch" }
    G@{ shape: diam, label: "Is<br/>Confirmed" }
    H@{ shape: rect, label: "Dispatch" }
    I@{ shape: rect, label: "Send DispatchedNotification" }

    Start --> A
    A --> B
    B --> C
    C --> D
    D --> |External| E
    D --> |Auto| F
    F --> G
    G --> |Yes| H
    G --> |No| J
    H --> I
```

#### <a name="subprocess-check-availability"></a>Check availability

Check availability sub process


#### <a name="subprocess-select-available-coordinator"></a> Select available coordinator
```mermaid
---
title: Select available coordinator
---
flowchart TB
    Start@{ shape: sm-circ, label: "Start" }
    End@{ shape: fr-circ, label: "End" }
    F@{ shape: fr-rect, label: "<a href="#check-availability">Check availability</a>" }
    H@{ shape: diam, label: "Is available" }
    K@{ shape: win-pane, label: "Availability cache" }
    H --> |Yes| I
    H --> |No| D

    Start-->A
    A-->End
```


#### <a name="subprocess-gather-endorsements"></a>Gather endorsements

Given that a node has started acting as a coordinator and has accepted delegation of a transaction, when that transaction is assembled, then the coordinator sends endorsement requests to all required endorsers.
Once an endorsement response has been received from the minimum set of endorsers, then the transaction is marked as ready to dispatch and the  


#### <a name="subprocess-reevaluate-active-delegations"></a>Revaluate active delegations

If a sender node has detected any confusion in terms of which node it believes to be the current coordinator, then it should re-evaluate the status of all inflight transactions that have been delegated to a coordinator and decide whether to rescind that delegation and delegate to a different coordinator.
It should be noted that rescinding a delegation is a passive decision.  There is no obligation to notify the delegate that has happened because this needs to be possible in cases where the sender node has lost connection to the delegate.  If the delegate node does continue to coordinate the transaction, then assuming it is operating as per spec, it will not get further than requesting confirmation to dispatch. That confirmation will be denied. The assumption, baked into the protocol, is that all other nodes will likewise passively rescind delegations from that coordinator and reject dispatch confirmations so the whole graph of dependencies behind that will need to be re-assembled which will also be rejected and eventually all rescinded delegations will be abandoned.



## Key architectural decisions and alternatives considered

### Implicit or explicit liveness detection

**Context**
It may be possible to detect liveness of nodes implicitly through the normal expected cross network activity (or lack thereof) or explicitly through additional cross network messages specifically for the purpose of liveness monitoring.

**Decision**
Explicit heartbeat messages are factored into the protocol.

**Consequences**
 - This is more provably functional and has similarities with other well known consensus algorithms such as RAFT and so has a better chance of being understood by more people which makes the overall protocol more susceptible to peer review and improvement.
 - Compared to implicitly detecting liveness, the explicit messages cause higher network usage but that is justified by the simplicity that it brings to the protocol.

**Status**
Proposal.  This decision has been discussed with other maintainers but is pending final acceptance.

### When to send heartbeat messages

**Context** 
Given the decision to use heartbeat messages for liveness detection, we need to decide which node(s) send heartbeat messages and under what conditions.  Should all nodes always broadcast heartbeats? Or only certain nodes at certain times?

**Decision**
Nodes only send heartbeat messages while they are a coordinator for an active contract i.e. while there are transactions in flight that have been delegated to that node.

**Consequences**
 - It is very likely that there are huge numbers of contracts in any given paladin network and most of them will be inactive ( will have zero transactions in flight) at any one time so it would be extremely wasteful and place huge burden on the network if potential coordinators for all contracts were to publish heartbeat messages proactively.
 - For the first transaction after an inactive period, the sender node has no knowledge of whether the current preferred coordinator is live.  The handshake for delivering the delegation message is the first opportunity to test liveness so that handshake must include an acknowledgement leg.
 - When a node restarts, if it is the preferred coordinator for an active contract, it does not necessary know that contract is active. While the node was down, all senders are likely to have chosen an alternative coordinator and will continue to delegate all transactions to it.  The first opportunity for a newly started node ( or a node that was on an isolated network partition that has been reconnected) would be when it receives the heartbeat messages from the current coordinator.  Thus, when any node receives a heartbeat message from a coordinator, it should trigger the node to evaluate whether it believes itself is a more preferred coordinator for that contract at this point in time and start sending heartbeat messages if so.

**Status**
Proposal.  This decision has been discussed with other maintainers but is pending final acceptance.

### Assurance of exactly once submission per intent

**Context**
An `intent` is a request, from a paladin user, to invoke a private transaction. This request is persisted to the database on the `sender` node before returning success to the user.  This `intent` must be finalized by either a) marking the transaction as `reverted` with a revert reason or b) confirming exactly one transaction on the base ledger that records the states spent and the new states produced by fulfilling this `intent`.  Various errors can occur while preparing and submitting the transaction to the base ledger and so the distributed paladin engine performs retry processing to ensure a valid transaction `intent` does eventually result in a confirmed base ledger transaction.  The nature of the distributed processing and retry logic means that we need an explicit architectural decision about how to assure that `intents` are fulfilled at most once by a base ledger transaction.

**Decision**
The distributed processing of the paladin engine aims to minimize the probability of double submission but full assurance is only provided by validation on the base ledger contract.

**Consequences**
 - This means that there is an extra gas cost for every transaction and the implementation of the base ledger contract needs to find the most efficient way of performing this validation e.g. using sparse merkle tree.
 - The alternative would be to design a coordinated transactional handshake that was resilient to network outage.  There are known protocols ( such as 2 phase commit XA protocols) that could give us assurance that the prepared transaction eventually exists exactly once in one submitters database but that does not guarantee that it will end up successfully submitted to the base ledger and may lead to a stranded transaction.  So, if we did adopt that approach, we would need to mitigate the stranded transaction situation with a timeout / retry on the part of the sender.  If that retry turned out to be too eager, then we would still end up with duplicate submission to the base ledger.
 
 **Status**
Proposal.  This decision has been discussed with other maintainers but is pending final acceptance.

### Majority vote for confirmation of non availability of preferred coordinator

**Context**
Given that all nodes can reach an independent universal conclusion about the ranking of preferred coordinators for any given point in time, the actual choice of coordinator depends on which of the preferred coordinators happen to be available.  The awareness of availability is not something that can be deterministically calculated because each node may have different information.  A decision needed to be made about whether this algorithm depends on all nodes (or a majority of nodes) agreeing about availability of preferred coordinator.

**Decision**

Algorithm does *not* depend on nor provide a facility to ensure that all nodes reach the same conclusion regarding available of the preferred coordinator.

**Consequences**
 - algorithm is simpler and does not need a voting handshake or voting `term` counting
 - network partitions (where some nodes recognize one coordinator and other nodes recognize another coordinator) can happen.  To have an algorithm that completely avoids  network partitions would become very complex, and error prone.  When network conditions, that could cause a partition arise, then such an algorithm would grind to a halt.  Given that the algorithm's main objective is efficiency and we rely on the base ledger contract for correctness and transactional integrity, the preferred algorithm should be able to continue to function - albeit with reduced efficiency due to increase in error handling and retry - in the case of a partition.
