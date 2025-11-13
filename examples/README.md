# Paladin Examples

These examples demonstrate the various areas of functionality provided by Paladin.

Every example has its own README and setup instructions. The following setup is common to most examples and sets up the SDKs and common requirements ready to run any of the examples.

See the [tutorials](https://LFDT-Paladin.github.io/paladin/head/tutorials/) for more information.

## Prerequisites

### Set up a local Paladin network

All of the samples require a local 3-node Paladin network running on `localhost:31548`, `localhost:31648`, and `localhost:31748`. You can get one set up by referring to the [Getting Started](https://LFDT-Paladin.github.io/paladin/head/getting-started/installation/) guide.

This should provision a local Kubernetes (based on Kind) cluster and provision a Besu and Paladin network including these pods:

```shell
$ kubectl get po
NAME                               READY   STATUS    RESTARTS   AGE
besu-node1-0                       1/1     Running   0          19m
besu-node2-0                       1/1     Running   0          19m
besu-node3-0                       1/1     Running   0          19m
paladin-node1-0                    2/2     Running   0          17m
paladin-node2-0                    2/2     Running   0          17m
paladin-node3-0                    2/2     Running   0          17m
paladin-operator-bc788db4f-mzbs7   1/1     Running   0          19m
```

### Build the common package

```shell
cd <paladin-root>/examples/common
npm install                        # install dependencies
npm install @lfdecentralizedtrust-labs/paladin-sdk@latest
npm run download-abi               # download ABIs
npm run build                      # build the 'common' package
```

## Running Examples

### Local Development (Default)

Most examples can be run using the default local configuration:

```sh
cd <paladin-root>/examples/<example>
npm install
npm install @lfdecentralizedtrust-labs/paladin-sdk@latest
npm run copy-abi
npm run start
```

> **Note:** Some examples may require slightly different setup steps. Refer to the individual example's README for details.

### Remote Environment

To run an example against a remote Paladin network, provide a custom configuration file:

```sh
cd <paladin-root>/examples/<example>
npm install
npm install @lfdecentralizedtrust-labs/paladin-sdk@latest
npm run copy-abi
npm run start -- -c /path/to/your/config.json
```

For guidance on how to create the configuration file, see the [common README](common/README.md#custom-configuration).
