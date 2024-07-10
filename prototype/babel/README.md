To run this prototype

 - build the gable library
```
pushd ../gable
make
popd
```

 - copy the gable library to the current directory
```
cp ../gable/gable.so .
```

 - run this project
```
./gradlew run
```

If you see a message such as the following then the Java code has succesfully sent a message to the Go code
```
time="..." level=info msg="Received event id:\"...\" type:\"ping\" [ping]" contractId=
```

If you see a message such as the following then Java has recieved a response from the Go code

```
Response in Java ... [ack] to ... 
```

