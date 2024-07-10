# Portara

This is the main process for paladin.  To run this, you will need to have the kata library built and in the same directory as the jar file.  The kata library is a Go library that is used to orchestrate a transaction though assembly, endorsement, notarisation, and submission to the EVM base ledger.  The kata library is built using the following commands:

 - build the kata library
```
pushd ../kata
make
popd
```

 - run this project
```
./gradlew run
```

If you see a message such as the following then the Java code has succesfully sent a message to the Go code to submit a transaction with all the correct fields
```
time="..." level=info msg="Received SubmitTransactionRequest: contractAddress=0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa, from=0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb, idempotencyKey=..., payload=&{{\"method\":\"foo\",\"params\":[\"bar\",\"quz\"]}}"
```

If you see a message such as the following then Java has recieved a response from the Go code with the transaction id
```
Transaction submitted: your-transaction-id
```

Hit Ctrl-C to stop the process.
