# Spec compliance tests

## Sender tests

### Initial state
`WHEN` a user calls `ptx_SendTransaction`
`THEN` a new transaction is created in the `Pending` state
  - `AND` the transaction id is returned to the user

### Pending state
`GIVEN` that a transaction is in the `Pending` state
`WHEN` the sender node is restarted
`THEN` the transaction is still in the `Pending` state

`GIVEN` that a transaction is in the `Pending` state
`WHEN` a user calls `ptx_getTransaction` with that transaction's id
`THEN` the response includes the details of the transaction
  `AND` does not include a receipt

`GIVEN` that a transaction is in the `Pending` state
 `AND` it has no dependencies
`WHEN` <!-- TODO What is the trigger? Or should we have a `WITHIN <duration>` or `EVENTUALLY`?  `EVENTUALLY` is difficult to disprove -->
`THEN` the sender sends a `DelegationRequest` to the preferred coordinator
 `AND` the transaction is in the `Delegating` state

`GIVEN` that a transaction is in the `Pending` state
`WHEN` the sender receives a `SubmitterHeartbeat` message that includes that transaction's id
`THEN` the transaction is in `Dispatched` state

`GIVEN` the transaction is in the `Pending` state
 `AND` `DelegationAttemptTime`  is the time that the `DelegationRejected` message was received
 `AND` `DelegationResult` is  `Rejected`
 `AND` `DelegationRejectionReason` is  `CoordinatorBehind`
`WHEN` `DelegationRetryInterval` passed
`THEN` the sender sends a `DelegationRequest` to the preferred coordinator
 `AND` the transaction is in the `Delegating` state

`GIVEN` the transaction is in the `Pending` state
 `AND` `DelegationAttemptTime`  is the time that the `DelegationRejected` message was received
 `AND` `DelegationResult` is  `Rejected`
 `AND` `DelegationRejectionReason` is  `CoordinatorAhead`
`WHEN` a new block height is detected which moves the sender's block height to a new block range
`THEN` the sender sends a `DelegationRequest` to the preferred coordinator
 `AND` the transaction is in the `Delegating` state


### Delegating state

`GIVEN` that a transaction is in the `Delegating` state
 `AND` the sender has sent a `DelegationRequest` 
`WHEN` a `DelegationAccepted` message is received where the `DelegationID` matches the `DelegationRequest` message
`THEN` the transaction is in the `Delegated` state

`GIVEN` that a transaction is in the `Delegating` state
 `AND` the sender has sent a `DelegationRequest` 
`WHEN` a `DelegationAccepted` message is received where the `DelegationID` does not match a `DelegationRequest` message
`THEN` the transaction remains in the `Delegating` state

`GIVEN` that a transaction is in the `Delegating` state
 `AND` the sender has sent a `DelegationRequest` 
`WHEN` a `DelegationRejected` message is received where the `DelegationID` matches the `DelegationRequest` message and the block height of the coordinator is in a previous range to the sender
`THEN` the transaction is in the `Pending` state
 `AND` `DelegationAttemptTime`  is the time that the `DelegationRejected` message was received
 `AND` `DelegationResult` is  `Rejected`
 `AND` `DelegationRejectionReason` is  `CoordinatorBehind`

`GIVEN` that a transaction is in the `Delegating` state
 `AND` the sender has sent a `DelegationRequest` 
`WHEN` a `DelegationRejected` message is received where the `DelegationID` matches the `DelegationRequest` message and the block height of the coordinator is in a later range to the sender
`THEN` the transaction is in the `Pending` state
 `AND` `DelegationAttemptTime`  is the time that the `DelegationRejected` message was received
 `AND` `DelegationResult` is  `Rejected`
 `AND` `DelegationRejectionReason` is  `CoordinatorAhead`



### Delegated state

`GIVEN` that a transaction is in the `Delegated` state
`WHEN` a user calls `ptx_getTransaction` with that transaction's id
`THEN` the response includes the details of the transaction
  `AND` includes the current delegate node

`GIVEN` that a transaction is in the `Delegated` state
`WHEN` the sender node is restarted
`THEN` the transaction is in the `Pending` state

`GIVEN` that a transaction is in the `Delegated` state
`WHEN` the sender receives a `DispatchConfirmationRequest` 
  `WHERE` the `DelegateNode` matches the current delegate node for that transaction
`THEN` the sender returns a `DispatchConfirmation` message
  `AND` the transaction is in `Prepared` state

`GIVEN` that a transaction is in the `Delegated` state
`WHEN` a new block height is detected which moves the sender's block height to a new block range
`THEN` the transaction is in the `Pending` state

`GIVEN` that a transaction is in the `Delegated` state
`WHEN` the sender does not receive any `CoordinatorHeartbeatNotification` containing that transaction id for a period of `CoordinatorHeartbeatFailureThreshold`
`THEN` the transaction is in the `Pending` state
<!-- TODO is this really 2 tests? 1) no heartbeat 2) heartbeat but does not contain the transaction id -->

`GIVEN` that a transaction is in the `Delegated` state
`WHEN` the sender receives `AssembleRequest`
`THEN` the transaction is in the `Assembling` state
 `AND` the domain `AssembleTransaction` function is invoked


### Assembling state

`GIVEN` that a transaction is in the `Assembling` state
`WHEN` the domain `AssembleTransaction` function returns `AssembleTransactionResponse_OK` 
`THEN` the transaction is in the `Delegated` state
  `AND` the sender sends an `AssembleResponse` message to the coordinator

`GIVEN` that a transaction is in the `Assembling` state
`WHEN` the domain `AssembleTransaction` function returns `AssembleTransactionResponse_PARK` 
`THEN` the transaction is in the `Pending` state
  `AND` <!-- TODO define status for parked. Should this be a separate state to `Pending`?-->

`GIVEN` that a transaction is in the `Assembling` state
`WHEN` the domain `AssembleTransaction` function returns `AssembleTransactionResponse_REVERT` 
`THEN` the transaction is in the `Reverted` state

### Prepared state

`GIVEN` that a transaction is in the `Prepared` state
`WHEN` the sender receives a `SubmitterHeartbeat` message that includes that transaction's id
`THEN` the transaction is in `Dispatched` state

`GIVEN` that a transaction is in the `Prepared` state
`WHEN` the sender does not receive any `SubmitterHeartbeatNotification` containing that transaction id for a period of `SubmitterHeartbeatFailureThreshold`
`THEN` the transaction is in `Pending` state
<!-- TODO is this really 2 tests? 1) no heartbeat 2) heartbeat but does not contain the transaction id -->


### Dispatched state

`GIVEN` that a transaction is in the `Dispatched` state
`WHEN` a user calls `ptx_getTransactionFull` with that transaction's id
`THEN` the response includes the details of the transaction
  `AND` includes the current submitter information node

`GIVEN` that a transaction is in the `Dispatched` state
`WHEN` sender node reads a blockchain event for that transaction
`THEN` the transaction is in `Confirmed` state

`GIVEN` that a transaction is in the `Dispatched` state
`WHEN` sender node receives a `BaseLedgerRevert` notification from the submitter
`THEN` the transaction is in the `Pending` state

## Coordinator tests

## Submitter tests
