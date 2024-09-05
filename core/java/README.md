# Core Java

This is the main process for paladin.  To run this, you will need to have the core library built and in the same directory as the jar file.  The core library is a Go library that is used to orchestrate a transaction through assembly, endorsement, notarisation, and submission to the EVM base ledger.  The core library is built using the following commands:

 - build the core library
```
pushd ../go
make
popd
```

 - copy the core library to the current directory
```
cp ../go/core.so .
```
TODO: LD_LIBRARY_PATH / LIBPATH do not seem to work on MacOS.  Need to find a way to set the library path for the JVM on MacOS.

 - run this project
```
./gradlew run
```

If you see a message such as the following then the Java code has succesfully sent a message to the Go code to submit a transaction with all the correct fields
```
time="..." level=info msg="Received SubmitTransactionRequest: contractAddress=0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa, from=0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb, idempotencyKey=..., payload=&{{\"method\":\"foo\",\"params\":[\"bar\",\"quz\"]}}"
```

If you see a message such as the following then Java has received a response from the Go code with the transaction id
```
Transaction submitted: your-transaction-id
```

Hit Ctrl-C to stop the process.
