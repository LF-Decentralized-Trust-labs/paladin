# Core component tests

## Run the whole test suite using gradle

Run tests with gas free chain:
```
gradle core:go:componentTestSQLite
```

Run tests with chain that uses gas:
```
gradle core:go:componentTestWithGasSQLite
```

# Core coordination tests

## Run the whole test suite using gradle

```
gradle core:go:coordinationTestPostgres
```

# Run in VS code

To run individual tests with the `Go` VS Code extension 

1. Start the test infrastructure
  ```
  gradle startTestInfra
  ```
1. Set `go.testEnvFile` to the absolute path to `core/go/noderuntests/.env` file

