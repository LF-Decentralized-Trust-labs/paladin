# Paladin Examples

These examples demonstrate the various areas of function provided by Paladin.

Every example has its own readme and setup instructions. The following setup is common to most examples and sets up the SDKs and common requirements ready to run any of the examples.

See the [tutorials](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/) for more information.

### Set up a local Paladin network

All of the samples require a local 3-node Paladin network running on `localhost:31548`, `localhost:31648`, and `localhost:31748`. You can get one setup by referring to the [Getting Started](https://lf-decentralized-trust-labs.github.io/paladin/head/getting-started/installation/) guide.

This should provision a local kubernetes (based on Kind) cluster and provision a Besu and Paladin network including these pods:

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
npm run download-abi               # download ABIs
npm run build                      # build the 'common' package
```
