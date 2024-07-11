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

import com.sun.jna.Native;
import com.sun.jna.Library;
import com.sun.jna.NativeLibrary;

public class PaladinJNA {

    private PaladinGo paladinGo;

    interface PaladinGo extends Library {
        int Run(String socketAddressPtr);
    }

    public PaladinJNA() {
        System.setProperty("jna.debug_load", "true");

        NativeLibrary.addSearchPath("kata","../kata");
        paladinGo = Native.load("kata", PaladinGo.class);
    }

    public int run(String socketAddress) {
        return paladinGo.Run(socketAddress);
    }
}