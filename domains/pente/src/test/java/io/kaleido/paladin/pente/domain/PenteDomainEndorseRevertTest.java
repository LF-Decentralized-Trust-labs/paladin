// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package io.kaleido.paladin.pente.domain;

import com.google.protobuf.ByteString;
import io.kaleido.paladin.toolkit.EndorsableState;
import io.kaleido.paladin.toolkit.EndorseTransactionRequest;
import io.kaleido.paladin.toolkit.EndorseTransactionResponse;
import org.junit.jupiter.api.Test;

import java.util.concurrent.ExecutionException;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

public class PenteDomainEndorseRevertTest {

    @Test
    void missingTransactionInputInfoStateReverts() throws ExecutionException, InterruptedException {
        var pente = new PenteDomain("", "");

        // Endorsement recovers the transaction from exactly one info state. Without it there is
        // nothing to re-execute, and no retry can change that — so this must revert, not error.
        var res = pente.endorseTransaction(EndorseTransactionRequest.newBuilder().build()).get();

        assertEquals(EndorseTransactionResponse.Result.REVERT, res.getEndorsementResult());
        assertTrue(res.getRevertReason().contains("Expected exactly one info state"),
                "revert reason should say what was wrong, got: " + res.getRevertReason());
    }

    @Test
    void tooManyTransactionInputInfoStatesReverts() throws ExecutionException, InterruptedException {
        var pente = new PenteDomain("", "");

        // The count check is an equality, not a lower bound: more than one info state is just as
        // ambiguous as none, and equally unfixable by re-asking.
        var res = pente.endorseTransaction(EndorseTransactionRequest.newBuilder()
                .addInfo(infoState("{}"))
                .addInfo(infoState("{}"))
                .build()).get();

        assertEquals(EndorseTransactionResponse.Result.REVERT, res.getEndorsementResult());
        assertTrue(res.getRevertReason().contains("Expected exactly one info state"),
                "revert reason should say what was wrong, got: " + res.getRevertReason());
    }

    @Test
    void unparseableTransactionInputInfoStateReverts() throws ExecutionException, InterruptedException {
        var pente = new PenteDomain("", "");

        // The info state carries the signed transaction we are being asked to re-execute. The same
        // bytes fail to parse the same way every time, so this reverts rather than erroring.
        var res = pente.endorseTransaction(EndorseTransactionRequest.newBuilder()
                .addInfo(infoState("not valid json"))
                .build()).get();

        assertEquals(EndorseTransactionResponse.Result.REVERT, res.getEndorsementResult());
        assertNotNull(res.getRevertReason());
    }

    private static EndorsableState infoState(String stateDataJson) {
        return EndorsableState.newBuilder()
                .setId(ByteString.copyFrom(new byte[32]).toString())
                .setSchemaId("schema1")
                .setStateDataJson(stateDataJson)
                .build();
    }
}
