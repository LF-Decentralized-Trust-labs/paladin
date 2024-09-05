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

package engine

import (
	"context"

	"github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

func NewPublisher(e *engine) enginespi.Publisher {
	return &publisher{
		engine: e,
	}
}

type publisher struct {
	engine *engine
}

// PublishStageEvent implements enginespi.Publisher.
func (p *publisher) PublishStageEvent(ctx context.Context, stageEvent *enginespi.StageEvent) error {

	p.engine.HandleNewEvent(ctx, stageEvent)
	return nil

}

// PublishEvent implements enginespi.Publisher.
func (p *publisher) PublishEvent(ctx context.Context, eventPayload interface{}) error {
	//TODO really need to decide when to use protobufs and when to use json
	// current assumption is that we would use golang structs for internal messages within a single engine,
	//protobuf for internal messages between nodes because it is faster and more efficient on the network bandwidth
	// and json for external messages because it is more consumable by external applications
	// but there are cases where all three of those are being emitted from the same point in the code
	// and we need to decide how to handle that
	log.L(ctx).Infof("Publishing event: %v", eventPayload)

	//TODO need to decide if we should leverage (or throwaway) the exising commsbus package.
	// for now, other than stage events, we only publish to subscribers of the engine and there is exactly one of those
	// at time of writing - which is the unit test
	// there may be a future where we want to publish to multiple internal subscribers, in which case we would need to use commsbus
	// and/or multiple external subscribers
	p.engine.publishToSubscribers(ctx, eventPayload)
	return nil
}
