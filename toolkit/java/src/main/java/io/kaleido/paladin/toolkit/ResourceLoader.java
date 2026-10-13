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

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.IOException;
import java.io.InputStream;
import java.io.StringWriter;

public class ResourceLoader {

    // ObjectMapper is thread-safe once configured, and is shared so its serializer cache is reused across calls.
    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

    public static String jsonResourceEntryText(ClassLoader classLoader, String resourcePath, String entry) throws IOException {
        try (InputStream is = classLoader.getResourceAsStream(resourcePath)) {
            if (is == null) {
                throw new IllegalArgumentException("resource %s not found".formatted(resourcePath));
            }
            JsonNode node = OBJECT_MAPPER.readTree(is);
            JsonNode entryNode = node.get(entry);
            if (entryNode == null) {
                throw new IllegalArgumentException("entry %s not found in JSON resource %s".formatted(entry, resourcePath));
            }
            if (entryNode.isValueNode()) {
                return entryNode.asText();
            }
            return OBJECT_MAPPER.writeValueAsString(entryNode);
        }
    }

}
