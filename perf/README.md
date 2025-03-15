# Paladin Performance CLI

Paladin Performance CLI is a HTTP load testing tool that generates a constant request rate against a Paladin node and measures performance. This is used to confirm confidence that a Paladin node can perform under normal conditions for an extended period of time.

## Items Subject to Testing

- Public transaction submission

## Build

The `pldperf` CLI needs building before you can use it.

Run `gradle build` in the `perf` directory to build and install the `pldperf` command.

## Run

### Public contract

This test submits transactions which call the `set` method on a [`simplestorage`](https://github.com/kaleido-io/kaleido-js/blob/master/deploy-transact/contracts/simplestorage.sol) contract.

1. Create a configuration file for your test. See [`example-quick-start.yaml`](./config/example-quick-start.yaml) for an example and [`conf.go`](./internal/conf/conf.go) for all configuration options.
1. Deploy the `simplestorage` smart contract. Replace <key> in the following command with the the name of the key you wish to use, e.g. `key@node1`.
    ```
    curl --location '127.0.0.1:31548' \
    --header 'Content-Type: application/json' \
    --data-raw '{
        "jsonrpc": "2.0",
        "id": "1",
        "method": "ptx_sendTransaction",
        "params": [{
            "type": "public",
            "abi": [
                {
                    "inputs": [],
                    "stateMutability": "nonpayable",
                    "type": "constructor"
                },
                {
                    "anonymous": false,
                    "inputs": [
                        {
                            "indexed": false,
                            "internalType": "uint256",
                            "name": "data",
                            "type": "uint256"
                        }
                    ],
                    "name": "DataStored",
                    "type": "event"
                },
                {
                    "inputs": [],
                    "name": "get",
                    "outputs": [
                        {
                            "internalType": "uint256",
                            "name": "retVal",
                            "type": "uint256"
                        }
                    ],
                    "stateMutability": "view",
                    "type": "function"
                },
                {
                    "inputs": [],
                    "name": "query",
                    "outputs": [
                        {
                            "internalType": "uint256",
                            "name": "retVal",
                            "type": "uint256"
                        }
                    ],
                    "stateMutability": "view",
                    "type": "function"
                },
                {
                    "inputs": [
                        {
                            "internalType": "uint256",
                            "name": "x",
                            "type": "uint256"
                        }
                    ],
                    "name": "set",
                    "outputs": [
                        {
                            "internalType": "uint256",
                            "name": "value",
                            "type": "uint256"
                        }
                    ],
                    "stateMutability": "nonpayable",
                    "type": "function"
                },
                {
                    "inputs": [],
                    "name": "storedData",
                    "outputs": [
                        {
                            "internalType": "uint256",
                            "name": "",
                            "type": "uint256"
                        }
                    ],
                    "stateMutability": "view",
                    "type": "function"
                }
            ],
            "bytecode": "6080604052348015600e575f80fd5b5060015f819055506102af806100235f395ff3fe608060405234801561000f575f80fd5b506004361061004a575f3560e01c80632a1afcd91461004e5780632c46b2051461006c57806360fe47b11461008a5780636d4ce63c146100ba575b5f80fd5b6100566100d8565b604051610063919061018f565b60405180910390f35b6100746100dd565b604051610081919061018f565b60405180910390f35b6100a4600480360381019061009f91906101d6565b6100e5565b6040516100b1919061018f565b60405180910390f35b6100c261016f565b6040516100cf919061018f565b60405180910390f35b5f5481565b5f8054905090565b5f60648210610129576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016101209061025b565b60405180910390fd5b815f819055507f9455957c3b77d1d4ed071e2b469dd77e37fc5dfd3b4d44dc8a997cc97c7b3d498260405161015e919061018f565b60405180910390a15f549050919050565b5f8054905090565b5f819050919050565b61018981610177565b82525050565b5f6020820190506101a25f830184610180565b92915050565b5f80fd5b6101b581610177565b81146101bf575f80fd5b50565b5f813590506101d0816101ac565b92915050565b5f602082840312156101eb576101ea6101a8565b5b5f6101f8848285016101c2565b91505092915050565b5f82825260208201905092915050565b7f56616c75652063616e206e6f74206265206f76657220313030000000000000005f82015250565b5f610245601983610201565b915061025082610211565b602082019050919050565b5f6020820190508181035f83015261027281610239565b905091905056fea26469706673582212200f06afa0bff2e5cf52b2437330e8c116fbccbb884dc359663cd63af0a7712e5464736f6c634300081a0033",
            "function": "",
            "from": <key>,
            "data": {}
        }
        ]
    }'
    ```
1. Get the address the contract is deployed to and set contract address in the configuration file to this value
    ```
        curl --location '127.0.0.1:31548' \
    --header 'Content-Type: application/json' \
    --data '{
        "jsonrpc": "2.0",
        "id": "1",
        "method": "ptx_getTransactionFull",
        "params": [<transaction id from deploying contract>]
    }'
    ```
1. Create a receipt listener for public transactions. This listener must be called `publiclistener` and can be used across multiple test runs.
    ```
    curl --location '127.0.0.1:31548' \
    --header 'Content-Type: application/json' \
    --data '{
        "jsonrpc": "2.0",
        "id": "1",
        "method": "ptx_createReceiptListener",
        "params": [
            {
                "name": "publiclistener",
                "filters": {
                    "type": "public"
                }
            }
        ]
    }'
    ```
1. Run the test
    ```
    pldperf run -c <config file> -i 0
    ```

## Command line options

```
Usage:
  pldperf run [flags]

Flags:
  -c, --config string          Path to performance config that describes the nodes and test instances
  -d, --daemon                 Run in long-lived, daemon mode. Any provided test length is ignored.
      --delinquent string      Action to take when delinquent messages are detected. Valid options: [exit log] (default "exit")
  -h, --help                   help for run
  -i, --instance-idx int       Index of the instance within performance config to run (default -1)
  -n, --instance-name string   Instance within performance config to run
```

## Metrics

The `pldperf` tool registers the following metrics for prometheus to consume:

- `pldperf_runner_actions_submitted_total`
- `pldperf_runner_received_events_total`
- `pldperf_runner_incomplete_events_total`
- `pldperf_runner_deliquent_msgs_total`
- `pldperf_runner_perf_test_duration_seconds`

## Useful features

The `pldperf` tool is designed to let you run various styles of test. There are various options for creating your own customized tests. A full list of configuration options can be seen at [`conf.go`](internal/conf/conf.go) but some useful options are outlined below:

- Setting a maximum number of test actions
  - See `maxActions` attribute (defaults to `0` i.e. unlimited).
  - Once `maxActions` test actions (e.g. token mints) have taken place the test will shut down.
- Ending the test when an error occurs
  - See `delinquentAction` attribute (defaults to `exit`).
  - A value of `exit` causes the test to end if an error occurs. Set to `log` to simply log the error and continue the test.
- Set the maximum duration of the test
  - See `length` attribute.
  - Setting a test instance's `length` attribute to a time duration (e.g. `3h`) will cause the test to run for that long or until an error occurs (see `delinquentAction`).
  - Note this setting is ignored if the test is run in daemon mode (running the `pldperf` command with `-d` or `--daemon`, or setting the global `daemon` value to `true` in the `instances.yaml` file). In daemon mode the test will run until `maxActions` has been reached or an error has occurred and `delinquentActions` is set to true.
- Ramping up the rate of test actions
  - See the `startRate`, `endRate` and `rateRampUpTime` attribute of a test instance.
  - All values default to `0` which has the effect of not limiting the rate of the test.
  - The test will allow at most `startRate` actions to happen per second. Over the period of `rateRampUpTime` seconds the allowed rate will increase linearly until `endRate` actions per seconds are reached. At this point the test will continue at `endRate` actions per second until the test finishes.
  - If `startRate` is the only value that is set, the test will run at that rate for the entire test.
- Waiting for events to be confirmed before doing the next submission
  - See `noWaitSubmission` (defaults to `false`).
  - When set to `true` each worker routine will perform its action (e.g. minting a token) and wait for confirmation of that event before doing its next action.
  - `maxSubmissionsPerSecond` can be used to control the maximum number of submissions per second to avoid overloading the system under test.
- Having a worker loop submit more than 1 action per loop by setting `actionsPerLoop` for the test. This can be helpful when you want to scale the number of actions done in parallel without having to scale the number of workers. The default value is `1` for this attribute. If setting to a value > `1` it is recommended to have `noWaitSubmission` to set `false`.
