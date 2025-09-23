# Core component tests

## Run the whole test suite using gradle

Run tests with gas free chain:
```
gradle core:go:componentTestPostgres
```

Run tests with chain that uses gas:
```
gradle core:go:componentTestWithGasPostgres
```

## Run in VS code

To run individual tests with the `Go` VS Code extension 

1. Start the test infrastructure
  ```
  gradle startTestInfra
  ```
1. Set `go.testEnvFile` to the absolute path to `core/go/componenttest/.env` file

