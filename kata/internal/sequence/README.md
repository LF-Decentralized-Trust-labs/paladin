# Sequence
The responsibility of this package is to determine which transactions belong to which sequence and which paladin node is responsible for dispatching the sequence.

The allocation of transaction to sequence is determined based on which other transactions it is dependant on (either explicitly as specified by the user or implicitly as a consequence of which state's it is trying to spend). This is a mutable association, until a certain point when the transaction is locked in to a given sequence.

Once a transaction has been confirmed on the base ledger, then transactions behind it in the sequence are candidates for moving to another sequence should that be more optimal for the network.

The sequence allocator implements a distributed, event driven algorithm to determine which transactions are dependant on which other transactions. 


## Sequence allocator

The sequence allocator can be thought of as a distributed state machine where the state in question is the associations between:
 - transactions and other transactions that they depend on
 - transactions and the sequence they belong to

events that the sequence allocator responds to are:
 - a new transaction is received
 - a transaction has been assembled
 - a transaction is confirmed

events that the sequence allocator can publish are:
 - a transaction is locked in to a sequence
 - a transaction is moved to a different sequence
 - a new sequence is created

There is a special case optimisation for transactions that lose the coin toss and the domain that assembled the transaction has declared that if it loses the coin toss then it would attempt to reassemble the transaction by spending the output of the winning transaction. 


## Test cases

To help define these test cases, lets assuming the following object model, terminology and definitions:

 - `state`
 - `meta-state`
   - `pending`
   - `confirmed`
   - `spent`
   - `spending`
   - `proposed`
   - `claimed`
 - `transaction`
 - `dispatcher` 
 - `sequence`
 - `paladin node`
 - `mint`
 - `minting transaction`

Transactions spend and mint states.  Each state can be spent only once. Transactions are organised into sequences and each sequence is allocated to a dispatcher for submission to the base ledger.  There is one dispatcher per domain instance per paladin node.

For simplicity, we explore these test cases as though the sequence allocator is a singleton (albeit distrubuted across a set of nodes).  In reality, there is one sequence allocator per instance of each domain but for this discussion, we assume that all domains are independant from each other therfore what holds for one domain, will hold for multiple concurrent and independant domains.

#### state meta-states

When a transaction is assembled and before it has been endorsed/verified, any output states of that transaction are in the `proposed` metastate.  `proposed` should be considered a volatile state and there are no assurances that will eventually become a spendable state.

Once a transaction has been endorsed / verified, the `proposed` output states move to the `pending` output state and the system can optimistically assume that state will eventually become spendable. New transaction may be assembled to spend states in `pending` so long as they are sequenced behind the transaction that is predicted to mint that state.

Once a transaction has been confirmed on the base ledger, its output states move to the `confirmed` meta state. `confirmed` states are availble for spending by transactions without any sequencing requirements.

When a transaction is assembled and before it has been endorsed/verified, any states that transaction intends to spend ( input to the transaction) are considered to be in the `claimed` state.  It is possible, and occassionally expected, that multiple transactions, assembled concurrently, could be associated with a `claimed` state. However, only one of those transactions should be endorsed/verified and submitted to the base ledger otherwise all but one will eventually be reverted by the double spend protection logic on the base ledger contract.

Once a transaction has been endorsed/verified, its input states are moved to the `spending` metastate. At this point, there is an expectation that state will eventually be spent by that transaction and therefore there will be no further attempt for other transactions to `claim` that state.

Once a transaction has been confirmed on the base ledger, its input states are moved to the `spent` metastate. 

### Macro level test cases
The following macro level test cases assert behaviour of the overall system which is a distributed network of concurrent nodes that are each running a sequence allocator.  As such, and counter to conventional GIVEN/WHEN/THEN style, these test cases can have multiple `When` clauses representing events that are happening concurrently on different nodes.

Given a set of `states` are in `confirmed` `meta-state`,

When a `transaction` is assembled  spend those states, 

Then it is allocated to a sequence of 1 and that sequence is assigned to the dispatcher on the same node that assembled the transaction.

---

Given a set of states are in pending state and have an associated "minter" transaction,

When a transaction is assembled to spend those states, 

Then it is allocated to the same sequence as the "minter" for those states and that sequence remains assigned to the dispatcher that it already is assigned to.

---

Given a state is in confirmed meta-state

When 2 transactions are assembled concurrently to spend that state 

Then one transaction is allocated to a sequence of 1 and that sequence is assigned to the dispatcher on the same node that assembled that transaction and the other transaction is moved to the unassmebled state.

---

NOTE: there is a special case optimisation where the losing transaction may get first refusal - or at least has weighted probability to win the next coin toss - on the output of the winning transaction but we will discuss that separately.

### Micro level test cases
In order for the above macro level test cases to pass, the following micro level test cases must also pass. Each of these is written in the context of a single sequence allocator running on a single node.

Given that a transaction is assembled to spend a state that is in in the confirmed meta-state

When the assemble transaction event is detected

Then a new sequence is created and the transaction is added to it and the sequence is assigned to the dispatcher for the node that assembled the transaction and the state is moved to the `claimed` metastate

---

Given that a state is in the claimed state on the local node

When a transactionAssembled event is received from a remote node attempting to claim the same state

Then a determinisitic and fair coin toss will elect one of the transactions as the `spender` of that state 

---

Given that a state is in the claimed state on the local node

When a transactionEndorsed event is received

Then the transaction is updated with an incremented number of endorsers and if endorsement rules are fulfilled, then the transaction is marked as `endorsed` and all of its `claimed` intput states are marked as `spending`

### Properties of the coin toss function
Inputs are 
 - the unique id of the state being contested
 - the unique ids of 2 transactions attempting to claim the state

Output is
 - id of one of the 2 transactions

Behaviour
 - for any 2 given transaction ids and statistically significant set of randomly generated state ids, when invoked for each state id, it returns one of the transaction ids for near 50% of the state IDs and the other transaction id for near 50% of the state IDs
 - Is a pure function. For same inputs, will always return the same output, even if executed multiple times on completely separate nodes in the network 
 - Is commutative over the transaction ids. If invoked with the same 2 transaction ids (and same state id) will return the same answer regardless of the order of the transaction ids
 - Is associative.  When more than 2 transactions are contesting for the same state id, the coin toss can be invoked multiple times, taking the result of one toss as one of the inputs to the next invocation untill all transactions have been included in at least one coin toss, then the final winner will always be the same regardless of which order the transactions were included in the coin toss.  
  
To explain these properties further.  Imagine there are 4 transactions contesting for the same state ID, lets call them `Ta`, `Tb`, `Tc` and `Td`. And lets write the coin toss between `Ta` and `Tb` for state `S` as `C(S,Ta,Tb)`
  
  `C(S,Ta,Tb) === C(S,Tb,Ta)` as per the commutative property
  
We can perform any of the following sequences of operation and will always get the same answer as per the associative property:   
 - `C(S,(C(S,Ta,Tb)),(C(S,Tc,Td))` : 1 toss up between a and b, another toss up between c and d, then a final toss up between the 2 winners.
 - `C(S,C(S,C(S,Ta,Tb),Tc),Td)`: 1 toss up between a and b. Another toss up between c and the winner of the first toss. A third toss up between d and the winner of the second toss up.

