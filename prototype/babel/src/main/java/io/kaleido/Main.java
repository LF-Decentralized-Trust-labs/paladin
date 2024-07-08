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
package io.kaleido;

import java.io.File;
import java.io.IOException;

public class Main {
    public static void main(String[] args) throws IOException {
        File f = File.createTempFile("paladin", ".sock");
        if (!f.delete() ){
            throw new IOException(String.format("Failed to deleted socket placeholder after creation: %s", f.getAbsolutePath()));
        }
        int rc = new PaladinJNI().run(f.getAbsolutePath());
        if (rc != 0) {
            throw new IOException("Failed to start golang gRPC server");
        }
    }
}