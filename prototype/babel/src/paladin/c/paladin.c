#include <jni.h>
#include <stdio.h>

JNIEXPORT void JNICALL
Java_io_kaleido_PaladinJNI_run(JNIEnv *env, jobject obj)
{
    printf("Hello From C++ World!\n");
    return;
}