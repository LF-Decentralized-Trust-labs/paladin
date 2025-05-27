/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package privatetxnmgr

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

func (tf *transactionFlow) logActionDebug(ctx context.Context, msg string) {
	//centralize the debug logging to force a consistent format and make it easier to analyze the logs
	log.L(ctx).Debugf("transactionFlow:Action TransactionID='%s' Status='%s' LatestEvent='%s' LatestError='%s' : %s", tf.transaction.ID, tf.status, tf.latestEvent, tf.latestError, msg)
}

func (tf *transactionFlow) logActionDebugf(ctx context.Context, msg string, args ...interface{}) {
	//centralize the debug logging to force a consistent format and make it easier to analyze the logs
	log.L(ctx).Debugf("transactionFlow:Action TransactionID='%s' Status='%s' LatestEvent='%s' LatestError='%s' : %s", tf.transaction.ID, tf.status, tf.latestEvent, tf.latestError, fmt.Sprintf(msg, args...))
}

func (tf *transactionFlow) logActionInfo(ctx context.Context, msg string) {
	//centralize the info logging to force a consistent format and make it easier to analyze the logs
	log.L(ctx).Infof("transactionFlow:Action TransactionID='%s' Status='%s' LatestEvent='%s' LatestError='%s' : %s", tf.transaction.ID, tf.status, tf.latestEvent, tf.latestError, msg)
}

func (tf *transactionFlow) logActionInfof(ctx context.Context, msg string, args ...interface{}) {
	//centralize the debug logging to force a consistent format and make it easier to analyze the logs
	log.L(ctx).Infof("transactionFlow:Action TransactionID='%s' Status='%s' LatestEvent='%s' LatestError='%s' : %s", tf.transaction.ID, tf.status, tf.latestEvent, tf.latestError, fmt.Sprintf(msg, args...))
}

func (tf *transactionFlow) logActionError(ctx context.Context, msg string, err error) {
	//centralize the info logging to force a consistent format and make it easier to analyze the logs
	log.L(ctx).Errorf("transactionFlow:Action TransactionID='%s' Status='%s' LatestEvent='%s' LatestError='%s' : %s.  %s", tf.transaction.ID, tf.status, tf.latestEvent, tf.latestError, msg, err.Error())
}

func (tf *transactionFlow) Action(ctx context.Context) {
	tf.statusLock.Lock()
	defer tf.statusLock.Unlock()

	tf.logActionDebug(ctx, ">>")
	if tf.complete {
		tf.logActionInfo(ctx, "Transaction is complete")
		return
	}

	// Lets get the nasty stuff out of the way first
	// if the event handler has marked the transaction as failed, then we initiate the finalize sync point
	if tf.finalizeRequired {
		if tf.finalizePending {
			tf.logActionInfo(ctx, "finalize already pending")
			return
		}
		//we know we need to finalize but we are not currently waiting for a finalize to complete
		// most likely a previous attempt to finalize has failed
		tf.finalize(ctx)
		tf.logActionInfo(ctx, "finalize initiated")
		return
	}

	if tf.dispatched {
		tf.logActionInfo(ctx, "Transaction is dispatched")
		return
	}

	if tf.transaction.PreAssembly == nil || tf.transaction.PreAssembly.TransactionSpecification == nil {
		tf.logActionDebug(ctx, "PreAssembly is nil")
		panic("PreAssembly is nil.")
		//This should never happen unless there is a serious programming error or the memory has been corrupted
		// PreAssembly is checked for nil after InitTransaction which is during the synchronous transaction request
		// and before it is added to the transaction processor / dispatched to the event loop
	}

	//depending on the delegation policy, we may be able to decide to delegate before we have assembled the transaction
	if !tf.delegateIfRequired(ctx) {
		tf.logActionDebug(ctx, "no continue after pre assembly delegate check")
		return
	}

	if tf.transaction.PostAssembly == nil {
		tf.logActionDebug(ctx, "PostAssembly is nil")

		//if we have not sent a request, or if the request has timed out or been invalidated by a re-assembly, then send the request
		tf.requestVerifierResolution(ctx)
		if tf.hasOutstandingVerifierRequests(ctx) {
			tf.logActionInfo(ctx, "Transaction not ready to assemble. Waiting for verifiers to be resolved")
			return
		}

		tf.requestAssemble(ctx)
		if tf.transaction.PostAssembly == nil {
			tf.logActionInfo(ctx, "Transaction not assembled. Waiting for assembler to return")
			return
		}
		if tf.transaction.PostAssembly.AssemblyResult == prototk.AssembleTransactionResponse_REVERT {
			log.L(ctx).Infof("Transaction %s reverted. Waiting for revert event to be processed", tf.transaction.ID.String())
			return
		}
	}

	// Must be signed on the same node as it was assembled so do this before considering whether to delegate
	tf.requestSignatures(ctx)
	if tf.hasOutstandingSignatureRequests() {
		return
	}
	tf.status = "signed"

	//depending on the delegation policy, we may not be able to decide to delegate until after we have assembled the transaction
	if !tf.delegateIfRequired(ctx) {
		tf.logActionDebug(ctx, "no continue after post assembly delegate check")
		return
	}

	log.L(ctx).Debugf("transactionFlow:Action TransactionID='%s' is ready for sequencing (outputStatesPotential=%d outputStates=%d)",
		tf.transaction.ID.String(), len(tf.transaction.PostAssembly.OutputStatesPotential), len(tf.transaction.PostAssembly.OutputStates))

	//If we get here, we have an assembled transaction and have no intention of delegating it
	// so we are responsible for coordinating the endorsement flow
	// either because it was submitted locally and we decided not to delegate or because it was delegated to us

	tf.requestEndorsements(ctx)
	if tf.hasOutstandingEndorsementRequests(ctx) {
		tf.logActionDebug(ctx, "Transaction not ready to dispatch. Waiting for endorsements to be resolved")
		return
	}
	tf.status = "endorsed"

	reDelegate, err := tf.setTransactionSigner(ctx)
	if err != nil {

		log.L(ctx).Errorf("Invalid outcome from signer selection %s: %s", tf.transaction.ID.String(), err)
		tf.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerResolveDispatchError), err.Error())

		//TODO as it stands, we will just enter a retry loop of trying to resolve the dispatcher next time the event loop triggers an action
		// if we are lucky, that will be triggered by an event that somehow changes the in memory state in a way that the dispatcher can be
		// resolved but that is unlikely
		// would it be more appropriate to re-assemble ( or even revert ) the transaction here?
		return
	} else if reDelegate {
		// TODO: We should re-delegate in this scenario
		tf.latestError = i18n.NewError(ctx, msgs.MsgPrivateReDelegationRequired).Error()
	}
	tf.logActionDebug(ctx, "<<")

}

func (tf *transactionFlow) setTransactionSigner(ctx context.Context) (reDelegate bool, err error) {
	// We only set the signing key in one very specific ENDORSER_MUST_SUBMIT path in this function.
	// In the general case the Sequencer picks a random signing key to submit the transaction.
	tx := tf.transaction
	tx.Signer = ""

	// We are the coordinator to be running this function.
	// We need to check if:
	// 1. There are any ENDORSER_MUST_SUBMIT constraints
	// 2. If there are, that we are the correct coordinator, or if we need to re-delegate.
	endorserSubmitSigner := ""
	for _, ar := range tx.PostAssembly.Endorsements {
		for _, c := range ar.Constraints {
			if c == prototk.AttestationResult_ENDORSER_MUST_SUBMIT {
				if endorserSubmitSigner != "" {
					// Multiple endorsers claiming it is an error
					return false, i18n.NewError(ctx, msgs.MsgDomainMultipleEndorsersSubmit)
				}
				log.L(ctx).Debugf("Endorser %s provided an ENDORSER_MUST_SUBMIT signing constraint for transaction %s", ar.Verifier.Lookup, tx.ID)
				endorserSubmitSigner = ar.Verifier.Lookup
			}
		}
	}
	if endorserSubmitSigner == "" {
		// great - we just need to use the anonymous signing management of the coordinator
		return false, nil
	}

	contractConf := tf.domainAPI.ContractConfig()
	if contractConf.SubmitterSelection != prototk.ContractConfig_SUBMITTER_COORDINATOR {
		// We only accept ENDORSER_MUST_SUBMIT constraints for contracts configured with coordinator submission.
		return false, i18n.NewError(ctx, msgs.MsgDomainEndorserSubmitConfigClash,
			endorserSubmitSigner, contractConf.CoordinatorSelection, contractConf.SubmitterSelection)
	}

	// Now we need to check the configuration for how the coordinator is picked
	switch contractConf.CoordinatorSelection {
	case prototk.ContractConfig_COORDINATOR_STATIC:
		staticCoordinator := ""
		if contractConf.StaticCoordinator != nil {
			staticCoordinator = *contractConf.StaticCoordinator
		}
		if endorserSubmitSigner != staticCoordinator {
			// If you have a static coordinator, and an endorser with an ENDORSER_MUST_SUBMIT, they must match.
			return false, i18n.NewError(ctx, msgs.MsgDomainEndorserSubmitConfigClash,
				endorserSubmitSigner, fmt.Sprintf(`%s='%s'`, contractConf.CoordinatorSelection, staticCoordinator),
				contractConf.SubmitterSelection)
		}
	case prototk.ContractConfig_COORDINATOR_ENDORSER:
		// This is fine, but it's possible we've ended up with the wrong coordinator/endorser combination.
	default:
		// This is invalid. In order for an endorsement to be able to provide an ENDORSER_MUST_SUBMIT
		// constraint it must be configured so we are allowed to pick the coordinator to be the endorser.
		return false, i18n.NewError(ctx, msgs.MsgDomainEndorserSubmitConfigClash,
			endorserSubmitSigner, contractConf.CoordinatorSelection, contractConf.SubmitterSelection)
	}

	// Ok we have a submission constraint to use the signing key of an endorser to submit.
	// Check it is a local identity. If not we have a re-delegation scenario.
	node, err := pldtypes.PrivateIdentityLocator(endorserSubmitSigner).Node(ctx, false /* must be fully qualified in this scenario */)
	if err != nil {
		return false, i18n.WrapError(ctx, err, msgs.MsgDomainEndorserSubmitConfigClash,
			endorserSubmitSigner, contractConf.CoordinatorSelection, contractConf.SubmitterSelection)
	}
	if node != tf.nodeName {
		log.L(ctx).Warnf("For transaction %s to be submitted, the coordinator must move to the node ENDORSER_MUST_SUBMIT constraint %s",
			tx.ID, endorserSubmitSigner)
		return true, nil
	}
	// Ok - we have an endorsement approval to use the returned
	// NON-ANONYMOUS identity homed on this local node to submit.
	tx.Signer = endorserSubmitSigner
	return false, nil
}

func (tf *transactionFlow) revertTransaction(ctx context.Context, revertReason string) {
	log.L(ctx).Errorf("Reverting transaction %s: %s", tf.transaction.ID.String(), revertReason)
	//trigger a finalize and update the transaction state so that finalize can be retried if it fails
	tf.finalizeRequired = true
	tf.finalizePending = true
	tf.finalizeRevertReason = revertReason
	tf.finalize(ctx)

}

func (tf *transactionFlow) finalize(ctx context.Context) {
	if tf.finalizeRevertReason == "" {
		log.L(ctx).Debugf("finalize transaction %s", tf.transaction.ID.String())

	} else {
		log.L(ctx).Errorf("finalize transaction %s: %s", tf.transaction.ID.String(), tf.finalizeRevertReason)
	}
	//flush that to the txmgr database
	// so that the user can see that it is reverted and so that we stop retrying to assemble and endorse it

	tf.syncPoints.QueueTransactionFinalize(
		ctx,
		tf.transaction.Domain,
		tf.domainAPI.Address(),
		tf.transaction.ID,
		tf.finalizeRevertReason,
		func(ctx context.Context) {
			//we are not on the main event loop thread so can't update in memory state here.
			// need to go back into the event loop
			log.L(ctx).Infof("Transaction %s finalize committed", tf.transaction.ID.String())

			// Remove this transaction from our domain context on success - all changes are flushed to DB at this point
			tf.domainContext.ResetTransactions(tf.transaction.ID)

			go tf.publisher.PublishTransactionFinalizedEvent(ctx, tf.transaction.ID.String())
		},
		func(ctx context.Context, rollbackErr error) {
			//we are not on the main event loop thread so can't update in memory state here.
			// need to go back into the event loop
			log.L(ctx).Errorf("Transaction %s finalize rolled back: %s", tf.transaction.ID.String(), rollbackErr)

			// Reset the whole domain context on failure
			tf.domainContext.Reset()

			go tf.publisher.PublishTransactionFinalizeError(ctx, tf.transaction.ID.String(), tf.finalizeRevertReason, rollbackErr)
		},
	)

	tf.finalizePending = true
}

func (tf *transactionFlow) delegateIfRequired(ctx context.Context) (doContinue bool) {

	if tf.delegatePending {
		tf.logActionInfof(ctx, "Transaction is delegating since %s (block=%d)", tf.delegateRequestTime, tf.delegateRequestBlockHeight)
		if tf.clock.Now().Before(tf.delegateRequestTime.Add(tf.requestTimeout)) {
			tf.logActionDebug(ctx, "Delegation request not timed out")
			return false
		}
		tf.logActionDebug(ctx, "Delegation request timed out")

	}

	if tf.status == "delegated" {
		// probably should not get here because the sequencer should have removed the transaction processor
		tf.logActionInfo(ctx, "Transaction is delegated")
		return false
	}

	// There may be a potential optimization we can add where, in certain domain configurations, we can optimistically proceed without delegation and only delegate once we detect
	// potential contention with other active nodes.  For now, we keep it simple and strictly abide by the configuration of the domain
	blockHeight, coordinatorNode, err := tf.selectCoordinator.SelectCoordinatorNode(ctx, tf.transaction, tf.environment)
	if err != nil {
		// errors from here are most likely a problem resolving the node name from the parties in the attestation plan
		// so there is no point retrying although if we redo the assemble stage, we may get a different result
		//TODO should we make the error action ( revert, reassemble, retry) an explicit property of the error so that this assessment can be made closer
		// to the source of the error?
		tf.latestError = err.Error()
		tf.transaction.PostAssembly = nil
		tf.logActionError(ctx, "Failed to select coordinator node", err)
		return false
	}

	// TODO persist the delegation and send the request on the callback
	if coordinatorNode == tf.nodeName || coordinatorNode == "" {
		// we are the coordinator so we should continue
		tf.logActionDebug(ctx, "Local coordinator")
		return true
	}
	tf.localCoordinator = false

	//TODO if already `delegating` check how long we have been waiting for the ack and send again.
	//Should probably do that earlier in the flow because if we have just decided not to delegate or if we have just selected a different delegate, \
	//then we need to either claw back that delegation or wait until the delegate has realized that they are no longer the coordinator and returns / forwards the responsibility for this transaction
	tf.status = "delegating"
	// ensure that the From field is fully qualified before sending to the delegate so that they can send assemble requests to the correct place
	fullQualifiedFrom, err := pldtypes.PrivateIdentityLocator(tf.transaction.PreAssembly.TransactionSpecification.From).FullyQualified(ctx, tf.nodeName)
	if err != nil {
		//this can only mean that the From field is an invalid identity locator and we should never have got this far
		// unless there is a serious programming error or the memory has been corrupted
		panic("Failed to fully qualify the coordinator node")
	}
	tf.transaction.PreAssembly.TransactionSpecification.From = fullQualifiedFrom.String()

	delegationRequestID := uuid.New().String()

	err = tf.transportWriter.SendDelegationRequest(
		ctx,
		delegationRequestID,
		coordinatorNode,
		tf.transaction,
		blockHeight,
	)
	if err != nil {
		tf.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), err.Error())
		tf.logActionError(ctx, "Failed to send delegation request", err)
	}
	tf.pendingDelegationRequestID = delegationRequestID
	tf.delegatePending = true
	tf.delegateRequestBlockHeight = blockHeight
	tf.delegateRequestTime = tf.clock.Now()
	tf.delegateRequestTimer = time.AfterFunc(tf.requestTimeout, func() {
		tf.publisher.PublishNudgeEvent(ctx, tf.transaction.ID.String())
	})
	//we have initiated a delegation so we should not continue any further with the flow on this node
	tf.logActionInfo(ctx, fmt.Sprintf("Delegating transaction to %s", coordinatorNode))
	return false

}

func (tf *transactionFlow) writeAndLockStates(ctx context.Context) {
	//this needs to be carefully coordinated with the assemble requester thread and the sequencer event loop thread
	// we are accessing the transactionFlow's PrivateTransaction object which is only safe to do on the sequencer thread
	// but we need to make sure that these writes/locks are complete before the assemble requester thread can proceed to assemble
	// the next transaction
	if (tf.transaction.PostAssembly.OutputStatesPotential != nil && tf.transaction.PostAssembly.OutputStates == nil) ||
		(tf.transaction.PostAssembly.InfoStatesPotential != nil && tf.transaction.PostAssembly.InfoStates == nil) {
		// We need to write the potential states to the domain before we can sign or endorse the transaction
		// but there is no point in doing that until we are sure that the transaction is going to be coordinated locally
		// so this is the earliest, and latest, point in the flow that we can do this
		readTX := tf.components.Persistence().NOTX() // no DB transaction required here for the reads from the DB (writes happen on syncpoint flusher)
		err := tf.domainAPI.WritePotentialStates(tf.domainContext, readTX, tf.transaction)
		if err != nil {
			//Any error from WritePotentialStates is likely to be caused by an invalid init or assemble of the transaction
			// which is most likely a programming error in the domain or the domain manager or privateTxManager
			// not much we can do other than revert the transaction with an internal error
			errorMessage := fmt.Sprintf("Failed to write potential states: %s", err)
			log.L(ctx).Error(errorMessage)
			//TODO publish an event that will cause the transaction to be reverted
			tf.revertTransaction(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), errorMessage))
			return
		} else {
			tf.logActionDebugf(ctx, "Potential states written %s", tf.domainContext.Info().ID)
		}
	}
	if len(tf.transaction.PostAssembly.InputStates) > 0 && tf.transaction.Intent == prototk.TransactionSpecification_SEND_TRANSACTION {
		readTX := tf.components.Persistence().NOTX() // no DB transaction required here for the reads from the DB (writes happen on syncpoint flusher)

		err := tf.domainAPI.LockStates(tf.domainContext, readTX, tf.transaction)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to lock states: %s", err)
			log.L(ctx).Error(errorMessage)
			tf.revertTransaction(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), errorMessage))
			return
		} else {
			tf.logActionDebugf(ctx, "Input states locked %s: %s", tf.domainContext.Info().ID, tf.transaction.PostAssembly.InputStates[0].ID)
		}
	}
}

func (tf *transactionFlow) requestAssemble(ctx context.Context) {
	//Assemble may require a call to another node ( in the case we have been delegated to coordinate transaction for other nodes)
	//Usually, they will get sent to us already assembled but there may be cases where we need to re-assemble
	// so this needs to be an async step
	// however, there must be only one assemble in progress at a time or else there is a risk that 2 transactions could chose to spend the same state
	//   (TODO - maybe in future, we could further optimize this and allow multiple assembles to be in progress if we can assert that they are not presented with the same available states)
	//   However, before we do that, we really need to sort out the separation of concerns between the domain manager, state store and private transaction manager and where the responsibility to single thread the assembly stream(s) lies

	log.L(ctx).Debug("transactionFlow:requestAssemble")

	if tf.transaction.PostAssembly != nil {
		tf.logActionDebug(ctx, "Already assembled")
		return
	}

	if tf.assemblePending {
		tf.logActionDebug(ctx, "Assemble already pending")
		return
	}

	var err error
	var assemblingNode string
	preAssemblyCopy := *tf.transaction.PreAssembly
	if preAssemblyCopy.TransactionSpecification == nil {
		err = i18n.NewError(ctx, msgs.MsgPrivateTxMgrAssembleRequestInvalid, tf.transaction.ID)
	}
	if err == nil {
		assemblingNode, err = pldtypes.PrivateIdentityLocator(preAssemblyCopy.TransactionSpecification.From).Node(ctx, true)
	}
	if err != nil {
		tf.publisher.PublishTransactionAssembleFailedEvent(
			ctx,
			tf.transaction.ID.String(),
			i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), "Failed to get node name from locator"),
			"",
		)
		return
	}

	tf.assembleCoordinator.QueueAssemble(
		ctx,
		assemblingNode,
		tf.transaction.ID,
		&preAssemblyCopy,
	)
	tf.assemblePending = true

}

func (tf *transactionFlow) requestSignature(ctx context.Context, attRequest *prototk.AttestationRequest, partyName string) {

	keyMgr := tf.components.KeyManager()

	unqualifiedLookup := partyName
	signerNode, err := pldtypes.PrivateIdentityLocator(partyName).Node(ctx, true)
	if signerNode != "" && signerNode != tf.nodeName {
		log.L(ctx).Debugf("Requesting signature from a remote identity %s for %s", partyName, attRequest.Name)
		err = i18n.NewError(ctx, msgs.MsgPrivateTxManagerSignRemoteError, partyName)
	}
	if err == nil {
		unqualifiedLookup, err = pldtypes.PrivateIdentityLocator(partyName).Identity(ctx)
	}
	var resolvedKey *pldapi.KeyMappingAndVerifier
	if err == nil {
		resolvedKey, err = keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, attRequest.Algorithm, attRequest.VerifierType)
	}
	if err != nil {
		log.L(ctx).Errorf("Failed to resolve local signer for %s (algorithm=%s): %s", partyName, attRequest.Algorithm, err)
		tf.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerResolveError), partyName, attRequest.Algorithm, err.Error())
		return
	}
	// TODO this could be calling out to a remote signer, should we be doing these in parallel?
	signaturePayload, err := keyMgr.Sign(ctx, resolvedKey, attRequest.PayloadType, attRequest.Payload)
	if err != nil {
		log.L(ctx).Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", partyName, resolvedKey.Verifier.Verifier, attRequest.Algorithm, err)
		tf.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerSignError), partyName, resolvedKey.Verifier.Verifier, attRequest.Algorithm, err.Error())
		return
	}
	log.L(ctx).Debugf("payload: %x signed %x by %s (%s)", attRequest.Payload, signaturePayload, partyName, resolvedKey.Verifier.Verifier)

	tf.publisher.PublishTransactionSignedEvent(ctx,
		tf.transaction.ID.String(),
		&prototk.AttestationResult{
			Name:            attRequest.Name,
			AttestationType: attRequest.AttestationType,
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       partyName,
				Algorithm:    attRequest.Algorithm,
				Verifier:     resolvedKey.Verifier.Verifier,
				VerifierType: attRequest.VerifierType,
			},
			Payload:     signaturePayload,
			PayloadType: &attRequest.PayloadType,
		},
	)
}

func (tf *transactionFlow) requestSignatures(ctx context.Context) {

	if tf.requestedSignatures {
		return
	}
	if tf.transaction.PostAssembly.Signatures == nil {
		tf.transaction.PostAssembly.Signatures = make([]*prototk.AttestationResult, 0)
	}
	attPlan := tf.transaction.PostAssembly.AttestationPlan
	attResults := tf.transaction.PostAssembly.Signatures

	for _, attRequest := range attPlan {
		switch attRequest.AttestationType {
		case prototk.AttestationType_SIGN:
			toBeComplete := true
			for _, ar := range attResults {
				if ar.GetAttestationType().Type() == attRequest.GetAttestationType().Type() {
					toBeComplete = false
					break
				}
			}
			if toBeComplete {

				for _, partyName := range attRequest.Parties {
					go tf.requestSignature(ctx, attRequest, partyName)
				}
			}
		}
	}
	tf.requestedSignatures = true
}

func (tf *transactionFlow) requestEndorsement(ctx context.Context, idempotencyKey string, party string, attRequest *prototk.AttestationRequest) {

	partyLocator := pldtypes.PrivateIdentityLocator(party)
	partyNode, err := partyLocator.Node(ctx, true)
	if err != nil {
		log.L(ctx).Errorf("Failed to get node name from locator %s: %s", party, err)
		tf.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), err.Error())
		return
	}

	if partyNode == tf.nodeName || partyNode == "" {
		// This is a local party, so we can endorse it directly
		endorsement, revertReason, err := tf.endorsementGatherer.GatherEndorsement(
			ctx,
			tf.transaction.PreAssembly.TransactionSpecification,
			tf.transaction.PreAssembly.Verifiers,
			tf.transaction.PostAssembly.Signatures,
			toEndorsableList(tf.transaction.PostAssembly.InputStates),
			toEndorsableList(tf.transaction.PostAssembly.ReadStates),
			toEndorsableList(tf.transaction.PostAssembly.OutputStates),
			toEndorsableList(tf.transaction.PostAssembly.InfoStates),
			party,
			attRequest)
		if err != nil {
			log.L(ctx).Errorf("Failed to gather endorsement for party %s: %s", party, err)
			tf.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), err.Error())
			return

		}
		tf.publisher.PublishTransactionEndorsedEvent(ctx,
			tf.transaction.ID.String(),
			idempotencyKey,
			party,
			attRequest.Name,
			endorsement,
			revertReason,
		)

	} else {
		// This is a remote party, so we need to send an endorsement request to the remote node

		err = tf.transportWriter.SendEndorsementRequest(
			ctx,
			idempotencyKey,
			party,
			partyNode,
			tf.transaction.Address.String(),
			tf.transaction.ID.String(),
			attRequest,
			tf.transaction.PreAssembly.TransactionSpecification,
			tf.transaction.PreAssembly.Verifiers,
			tf.transaction.PostAssembly.Signatures,
			tf.transaction.PostAssembly.InputStates,
			tf.transaction.PostAssembly.OutputStates,
			tf.transaction.PostAssembly.InfoStates,
		)
		if err != nil {
			log.L(ctx).Errorf("Failed to send endorsement request to party %s: %s", party, err)
			tf.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerEndorsementRequestError), party, err.Error())
		}
	}
}

func (tf *transactionFlow) requestEndorsements(ctx context.Context) {
	for _, outstandingEndorsementRequest := range tf.outstandingEndorsementRequests(ctx) {
		// there is a request in the attestation plan and we do not have a response to match it
		// first lets see if we have recently sent a request for this endorsement and just need to be patient
		previousRequestTime := time.Time{}
		idempotencyKey := uuid.New().String()
		previousIdempotencyKey := ""
		if pendingRequestsForAttRequest, ok := tf.pendingEndorsementRequests[outstandingEndorsementRequest.attRequest.Name]; ok {
			if r, ok := pendingRequestsForAttRequest[outstandingEndorsementRequest.party]; ok {
				previousRequestTime = r.requestTime
				previousIdempotencyKey = r.idempotencyKey
			}
		} else {
			tf.pendingEndorsementRequests[outstandingEndorsementRequest.attRequest.Name] = make(map[string]*endorsementRequest)
		}

		if !previousRequestTime.IsZero() && tf.clock.Now().Before(previousRequestTime.Add(tf.requestTimeout)) {
			//We have already sent a message for this request and the deadline has not passed
			log.L(ctx).Debugf("Transaction %s endorsement already requested %v", tf.transaction.ID.String(), previousRequestTime)
			return
		}
		if previousRequestTime.IsZero() {
			log.L(ctx).Infof("Transaction %s endorsement has never been requested for attestation request:%s, party:%s", tf.transaction.ID.String(), outstandingEndorsementRequest.attRequest.Name, outstandingEndorsementRequest.party)
		} else {
			log.L(ctx).Infof("Previous endorsement request for transaction:%s, attestation request:%s, party:%s sent at %v has timed out", tf.transaction.ID.String(), outstandingEndorsementRequest.attRequest.Name, outstandingEndorsementRequest.party, previousRequestTime)
		}
		if previousIdempotencyKey != "" {
			tf.logActionDebug(ctx, fmt.Sprintf("Previous endorsement request timed out. Sending new request with same idempotency key %s", previousIdempotencyKey))
			idempotencyKey = previousIdempotencyKey
		}

		tf.requestEndorsement(ctx, idempotencyKey, outstandingEndorsementRequest.party, outstandingEndorsementRequest.attRequest)
		tf.pendingEndorsementRequests[outstandingEndorsementRequest.attRequest.Name][outstandingEndorsementRequest.party] =
			&endorsementRequest{
				requestTime:    tf.clock.Now(),
				idempotencyKey: idempotencyKey,
			}

	}
}

func (tf *transactionFlow) requestVerifierResolution(ctx context.Context) {

	if tf.requestedVerifierResolution {
		log.L(ctx).Infof("Transaction %s verifier resolution already requested", tf.transaction.ID.String())
		return
	}

	//TODO keep track of previous requests and send out new requests if previous ones have timed out
	if tf.transaction.PreAssembly.Verifiers == nil {
		tf.transaction.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, 0, len(tf.transaction.PreAssembly.RequiredVerifiers))
	}
	// having duplicate requests for the same verifier can cause the same transaction to be sent multiple times
	// note that we leave the duplicates, if any, alone in the transaction object
	// and only dedup the requests that we send to the identity resolver
	requiredVerifiers := dedupResolveVerifierRequests(tf.transaction.PreAssembly.RequiredVerifiers)

	for _, v := range requiredVerifiers {
		tf.logActionDebugf(ctx, "Resolving verifier %s", v.Lookup)
		tf.identityResolver.ResolveVerifierAsync(
			ctx,
			v.Lookup,
			v.Algorithm,
			v.VerifierType,
			func(ctx context.Context, verifier string) {
				//response event needs to be handled by the sequencer so that the dispatch to a handling thread is done in fairness to all other in flight transactions
				tf.publisher.PublishResolveVerifierResponseEvent(ctx, tf.transaction.ID.String(), v.Lookup, v.Algorithm, verifier, v.VerifierType)
			},
			func(ctx context.Context, err error) {
				tf.publisher.PublishResolveVerifierErrorEvent(ctx, tf.transaction.ID.String(), v.Lookup, v.Algorithm, err.Error())
			},
		)
	}
	//TODO this needs to be more precise (like which verifiers have been sent / pending / stale  etc)
	tf.requestedVerifierResolution = true
}

func dedupResolveVerifierRequests(requests []*prototk.ResolveVerifierRequest) []*prototk.ResolveVerifierRequest {
	seen := make(map[string]struct{})
	var dedupedRequests []*prototk.ResolveVerifierRequest
	for _, request := range requests {
		key := fmt.Sprintf("%s:%s:%s", request.Lookup, request.VerifierType, request.Algorithm)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			dedupedRequests = append(dedupedRequests, request)
		}
	}
	return dedupedRequests
}
