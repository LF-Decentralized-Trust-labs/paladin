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

package io.kaleido.paladin.toolkit;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.IOException;
import java.io.Reader;
import java.io.StringReader;
import java.io.StringWriter;
import java.util.ArrayList;
import java.util.Arrays;

/** Serialization and de-serialization for ABIs */
@JsonIgnoreProperties(ignoreUnknown = true)
public class JsonABI extends ArrayList<JsonABI.Entry> {

        @JsonIgnoreProperties(ignoreUnknown = true)
        public record Entry(
                @JsonProperty
                String type,
                @JsonProperty
                @JsonInclude(JsonInclude.Include.NON_DEFAULT)
                String name,
                @JsonProperty
                @JsonInclude(JsonInclude.Include.NON_NULL)
                JsonABI.Parameters inputs,
                @JsonProperty
                @JsonInclude(JsonInclude.Include.NON_NULL)
                JsonABI.Parameters outputs,
                @JsonProperty
                @JsonInclude(JsonInclude.Include.NON_DEFAULT)
                String stateMutability,
                @JsonProperty
                @Deprecated
                @JsonInclude(JsonInclude.Include.NON_DEFAULT)
                boolean payable,
                @JsonProperty()
                @JsonInclude(JsonInclude.Include.NON_DEFAULT)
                @Deprecated
                boolean constant,
                @JsonProperty
                @JsonInclude(JsonInclude.Include.NON_DEFAULT)
                boolean anonymous
        ) {
                public String toString() {
                        return toJSON(false);
                }
                public String toJSON(boolean pretty) {
                        return JsonABI.toJSONString(this, pretty);
                }
        }

        @JsonIgnoreProperties(ignoreUnknown = true)
        public static class Parameters extends ArrayList<JsonABI.Parameter> {}

        @JsonIgnoreProperties(ignoreUnknown = true)
        public record Parameter(
                @JsonProperty
                String name,
                @JsonProperty
                String type,
                @JsonProperty
                @JsonInclude(JsonInclude.Include.NON_DEFAULT)
                String internalType,
                @JsonProperty
                @JsonInclude(JsonInclude.Include.NON_DEFAULT)
                JsonABI.Parameters components,
                @JsonProperty
                @JsonInclude(JsonInclude.Include.NON_DEFAULT)
                boolean indexed
        ) {
                public String toString() {
                        return toJSON(false);
                }
                public String toJSON(boolean pretty) {
                        return JsonABI.toJSONString(this, pretty);
                }
        }

        public String toString() {
                return toJSON(false);
        }

        public String toJSON(boolean pretty) {
                return JsonABI.toJSONString(this, pretty);
        }

        public static Entry newConstructor(Parameters inputs) {
                return new Entry("constuctor", "", inputs, null, "",false, false, false);
        }

        public static Entry newFunction(String name, Parameters inputs, Parameters outputs) {
                return new Entry("function", name, inputs, outputs, "",false, false, false);
        }

        public static Entry newEvent(String name, Parameters inputs) {
                return new Entry("event", name, inputs, null, "",false, false, false);
        }

        public static Parameters newParameters(Parameter ...inputs) {
                Parameters params = new Parameters();
                params.addAll(Arrays.asList(inputs));
                return params;
        }

        public static Parameter newParameter(String name, String type) {
                return new Parameter(name, type, "", null, false);
        }

        public static Parameter newIndexedParameter(String name, String type) {
                return new Parameter(name, type, "", null, true);
        }

        public static Parameter newTuple(String name, Parameters components) {
                return new Parameter(name, "tuple", "", components, false);
        }

        public static Parameter newTupleArray(String name, Parameters components) {
                return new Parameter(name, "tuple[]", "", components, false);
        }

        public static JsonABI fromJSONResourceEntry(ClassLoader classLoader, String resourcePath, String entry) throws IOException {
                String jsonABIText = ResourceLoader.jsonResourceEntryText(classLoader, resourcePath, entry);
                return fromString(jsonABIText);
        }

        public static JsonABI fromString(String abiJSON) throws IOException {
               return fromReader(new StringReader(abiJSON));
        }

        public static JsonABI fromReader(Reader abiJSON) throws IOException {
                ObjectMapper objectMapper = new ObjectMapper();
                return objectMapper.readValue(abiJSON, JsonABI.class);
        }

        private static String toJSONString(Object any, boolean pretty) {
                try {
                        ObjectMapper objectMapper = new ObjectMapper();
                        StringWriter out = new StringWriter();
                        if (pretty) {
                                return objectMapper.writerWithDefaultPrettyPrinter().writeValueAsString(any);
                        }
                        return objectMapper.writeValueAsString(any);
                } catch(IOException e) {
                        throw new RuntimeException(e);
                }
        }

        public JsonABI.Entry getABIEntry(String entryType, String entryName) {
                for (JsonABI.Entry abiEntry : this) {
                        if (abiEntry.type().equals(entryType) && abiEntry.name().equals(entryName)) {
                                return abiEntry;
                        }
                }
                throw new IllegalArgumentException("%s %s not found in ABI".formatted(entryType, entryName));
        }


}
