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

	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"google.golang.org/protobuf/proto"
)

func NewPublisher(engine Engine) types.Publisher {
	return &publisher{
		engine: engine,
	}
}

type publisher struct {
	engine Engine
}

// PublishEvent implements types.Publisher.
func (p *publisher) PublishStageEvent(ctx context.Context, stageEvent *types.StageEvent) error {

	p.engine.HandleNewEvents(ctx, stageEvent)
	return nil

}

// PublishEvent implements types.Publisher.
func (p *publisher) PublishEvent(ctx context.Context, eventPayload proto.Message) error {
	log.L(ctx).Infof("Publishing event: %v", eventPayload)
	return nil
}
