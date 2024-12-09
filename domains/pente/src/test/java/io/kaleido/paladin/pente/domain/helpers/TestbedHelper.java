package io.kaleido.paladin.pente.domain.helpers;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.testbed.Testbed;

import java.io.IOException;
import java.util.HashMap;

public class TestbedHelper {
    private static int POLL_INTERVAL_MS = 100;

    public static Testbed.TransactionResult getTransactionResult(HashMap<String, Object> res) {
        return new ObjectMapper().convertValue(res, Testbed.TransactionResult.class);
    }

    public static String sendTransaction(Testbed testbed, Testbed.TransactionInput input) throws IOException {
        return testbed.getRpcClient().request("ptx_sendTransaction", input);
    }

    public static Object getTransactionReceipt(Testbed testbed, String txID) throws IOException {
        return testbed.getRpcClient().request("ptx_getTransactionReceiptFull", txID);
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record TransactionReceipt(
            @JsonProperty
            String id,
            @JsonProperty
            boolean success,
            @JsonProperty
            String transactionHash,
            @JsonProperty
            long blockNumber
    ) {
    }

    public static TransactionReceipt pollForReceipt(Testbed testbed, String txID, int waitMs) throws IOException, InterruptedException {
        for (var i = 0; i < waitMs; i += POLL_INTERVAL_MS) {
            var receipt = getTransactionReceipt(testbed, txID);
            if (receipt != null) {
                return new ObjectMapper().convertValue(receipt, TransactionReceipt.class);
            }
            Thread.sleep(POLL_INTERVAL_MS);
        }
        return null;
    }
}
