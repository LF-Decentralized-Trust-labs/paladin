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
#include <jni.h>
#include <stdlib.h>
#include <stdio.h>
#include <libkata.h>
#include <cstring>

// Function to take a Java string, get the UTF-8 bytes from it, then malloc a char*
char* jstring2utf8(JNIEnv *env, jstring jStr) {
    if (!jStr) return (char *)"";

    const jclass stringClass = env->GetObjectClass(jStr);
    const jmethodID getBytes = env->GetMethodID(stringClass, "getBytes", "(Ljava/lang/String;)[B");
    const jbyteArray stringJbytes = (jbyteArray) env->CallObjectMethod(jStr, getBytes, env->NewStringUTF("UTF-8"));

    size_t length = (size_t) env->GetArrayLength(stringJbytes);
    jbyte* pBytes = env->GetByteArrayElements(stringJbytes, NULL);
    char* ret = (char *)malloc(length + 1);
    std::memcpy(ret, pBytes, length);
    ret[length] = 0;

    env->ReleaseByteArrayElements(stringJbytes, pBytes, JNI_ABORT);
    env->DeleteLocalRef(stringJbytes);
    env->DeleteLocalRef(stringClass);

    return ret;
}

extern "C" {

    JNIEXPORT jint JNICALL
    Java_io_kaleido_PaladinJNI_run(JNIEnv *env, jobject obj, jstring socketAddress)
    {
        char * pSocketAddress = jstring2utf8(env, socketAddress);
        printf("Running Golang with %s\n", pSocketAddress);
        int rc = Run(pSocketAddress); // from kata.h
        free(pSocketAddress);
        return rc;
    }

}
