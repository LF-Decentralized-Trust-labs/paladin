# paladin
EVM Privacy Stack

## How to run Paladin

The entry-point application of Paladin is under the [./runtime](./runtime/) directory.

You can start Paladin's JSON-RPC server as a Golang application:
```
go run runtime/main.go
```

You can use the following curl command to test the JSON-RPC endpoint:

```
curl -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method": "pld.SubmitTransaction", "params": [{"From": "Alice", "To": "Bob", "Amount": 10.5}], "id": 1}' http://localhost:1234/rpc
```

### Build Paladin docker image

Run `make docker` in the root or any subrepo directories.

## Repo structure
This is a [Monorepo](https://en.wikipedia.org/wiki/Monorepo). To avoid overcomplicating monorepo setup, you should adhere to the following practices:
1. One folder per repo. (a "repo" contains a set of code that could be separated into a standalone Github repo)
2. You can use folders to group repos, however, nested repos are not allowed.

## Github Action setup

- If adding a new library repo, you should add a Github Action using [./.github/workflows/library-build-template.yaml](./.github/workflows/library-build-template.yaml)