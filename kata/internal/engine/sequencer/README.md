# Sequence
The responsibility of this package is to determine which transactions belong to which sequence and which paladin node is responsible for dispatching the sequence.

The allocation of a transaction to sequence is determined based on which other transactions it is dependent on (either explicitly as specified by the user or implicitly as a consequence of which state it is trying to spend). This is a mutable association. When the transaction is locked into a given sequence, the association will become immutable unless events from the base ledger revoke the validity of the association. 

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

## Contention resolution
The sequencing of transactions is influenced heavily by the state(s) that each transaction has been assembled to spend.  Because the assembly of transactions happens concurrently, there are cases where 2 or more transactions are assembled to spend the same state.  This situation of contention must be resolved as a pre-req to the sequence allocation algorithm.  As a result of the contention resolution, exactly one of the the contenting transactions will proceed into a sequence with intent to spend that state. All other transactions are re-assembled to spend different states.

## Loop avoidance
Given that transactions can attempt to spend multiple states, there are situations possible where 2 transactions are contenting for the same 2 states.  The content resolution algorithm is sensitive to this and avoids resolving contention of one state in favour of one transaction and resolving contention of the other state in favour of the other transaction.  If this were to happen then neither transaction would proceed into a sequence.  Both would be re-assembled and there would be a potential for endless loop of assemble / contention resolution.  This is a trivial example of the loop scenarios but there are also more complex, less obvious situations, involving multiple transactions, that the algorithm is designed to avoid. We will explore these scenarios in more detail later as we enumerate the test cases that define the algorithm's functional behaviour.

## Deadlock avoidance
Given that transactions can be assembled to spend states that are predicted to exist in the future, once other transactions have been confirmed on the base ledger, the algorithm is designed to avoid the situation where one transaction is assembled to spend a state that is minted by another transaction while that  other transaction is also spending a state that is assembled to be spend a state that is being minted by the first transaction.

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

For simplicity, we explore these test cases as though the sequence allocator is a singleton (albeit distributed across a set of nodes).  In reality, there is one sequence allocator per instance of each domain but for this discussion, we assume that all domains are independent from each other therefore what holds for one domain, will hold for multiple concurrent and independent domains.

#### state meta-states

When a transaction is assembled and before it has been endorsed/verified, any output states of that transaction are in the `proposed` metastate.  `proposed` should be considered a volatile state and there are no assurances that will eventually become a spendable state.

Once a transaction has been endorsed / verified, the `proposed` output states move to the `pending` output state and the system can optimistically assume that state will eventually become spendable. New transaction may be assembled to spend states in `pending` so long as they are sequenced behind the transaction that is predicted to mint that state.

Once a transaction has been confirmed on the base ledger, its output states move to the `confirmed` meta state. `confirmed` states are available for spending by transactions without any sequencing requirements.

When a transaction is assembled and before it has been endorsed/verified, any states that transaction intends to spend ( input to the transaction) are considered to be in the `claimed` state.  It is possible, and occasionally expected, that multiple transactions, assembled concurrently, could be associated with a `claimed` state. However, only one of those transactions should be endorsed/verified and submitted to the base ledger otherwise all but one will eventually be reverted by the double spend protection logic on the base ledger contract.

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

Then one transaction is allocated to a sequence of 1 and that sequence is assigned to the dispatcher on the same node that assembled that transaction and the other transaction is moved to the unassembled state.

---

NOTE: there is a special case optimisation where the losing transaction may get first refusal - or at least has weighted probability to win the next contention resolver - on the output of the winning transaction but we will discuss that separately.


### Micro level test cases
In order for the above macro level test cases to pass, the following micro level test cases must also pass. Each of these is written in the context of a single sequence allocator running on a single node.

Given that a transaction is assembled to spend a state that is in in the confirmed meta-state

When the assemble transaction event is detected

Then a new sequence is created and the transaction is added to it and the sequence is assigned to the dispatcher for the node that assembled the transaction and the state is moved to the `claimed` metastate

---

Given that a state is in the claimed state on the local node

When a transactionAssembled event is received from a remote node attempting to claim the same state

Then a deterministic and fair contention resolver function will elect one of the transactions as the `spender` of that state 

---

Given that a state is in the claimed state on the local node

When a transactionEndorsed event is received

Then the transaction is updated with an incremented number of endorsers and if endorsement rules are fulfilled, then the transaction is marked as `endorsed` and all of its `claimed` input states are marked as `spending`

### Properties of the contention resolver function
Inputs are 
 - the unique id of the state being contested
 - the unique ids of 2 transactions attempting to claim the state

Output is
 - id of one of the 2 transactions

Behaviour
 - for any 2 given transaction ids and statistically significant set of randomly generated state ids, when invoked for each state id, it returns one of the transaction ids for near 50% of the state IDs and the other transaction id for near 50% of the state IDs
 - Is a pure function. For same inputs, will always return the same output, even if executed multiple times on completely separate nodes in the network 
 - Is commutative over the transaction ids. If invoked with the same 2 transaction ids (and same state id) will return the same answer regardless of the order of the transaction ids
 - Is associative.  When more than 2 transactions are contesting for the same state id, the contention resolver function can be invoked multiple times, taking the result of one invocation as one of the inputs to the next invocation until all transactions have been included in at least one invocation, then the final winner will always be the same regardless of which order the transactions were included.  
  
To explain these properties further.  Imagine there are 4 transactions contesting for the same state ID, lets call them `Ta`, `Tb`, `Tc` and `Td`. And lets write the contention resolver function between `Ta` and `Tb` for a given state `R(Ta,Tb)`
  
  `R(Ta,Tb) === R(Tb,Ta)` as per the commutative property
  
We can perform any of the following sequences of operation and will always get the same answer as per the associative property:   
 - `R(R(Ta,Tb),R(Tc,Td))` : 1 invocation between a and b, another invocation between c and d, then a final invocation between the 2 winners.
 - `R(R(R(Ta,Tb),Tc),Td)`: 1 invocation between a and b. Another invocation between c and the winner of the first invocation. A third invocation between d and the winner of the second invocation.

These properties are very important because the situation of `n` transactions contesting for the same state is an emerging situation and the knowledge of that situation ( which transactions are in contest) reaches different nodes in the network piecemeal, at different times and in a non deterministic order.  With the above properties, all nodes in the network will eventually reach the same answer regardless of what order each of them received the information.