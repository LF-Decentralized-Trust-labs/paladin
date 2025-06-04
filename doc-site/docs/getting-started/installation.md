# Installation

Installing the operator in `devnet` mode also installs and configures a three node Paladin network and three node Besu network. This is great way to get up and running quickly with Paladin and to try out the [tutorials](../tutorials/index.md).

## Pre-requisites

* Access to a running Kubernetes cluster
* [helm](https://helm.sh/) `v3` installed
* [kubectl](https://kubernetes.io/docs/reference/kubectl/) installed

## Quick Start with kind

If you are new to Kubernetes or simply need a quick way to get a running cluster, [kind](https://kind.sigs.k8s.io/)
provides a lightweight way to run a local Kubernetes cluster on your machine. This repo contains a
[starter config file](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/operator/paladin-kind.yaml)
for kind which will open up the container ports used by the Paladin charts below.

You can create a new cluster with:

```bash
curl https://raw.githubusercontent.com/LF-Decentralized-Trust-labs/paladin/refs/heads/main/operator/paladin-kind.yaml -L -O
kind create cluster --name paladin --config paladin-kind.yaml
```

If you need to start over, you can delete the cluster with:

```bash
kind delete cluster --name paladin
```

This guide is not intended to be a full tutorial on kind or Kubernetes - please refer to the relevant
documentation for any issues with these tools.

## Installation

### Step 1: Install the CRD Chart

Install the CRD chart that contains the necessary Custom Resource Definitions (CRDs) for the Paladin operator:

```bash
helm repo add paladin https://LF-Decentralized-Trust-labs.github.io/paladin --force-update
helm upgrade --install paladin-crds paladin/paladin-operator-crd
```

### Step 2: Install cert-manager CRDs

Install the [cert-manager](https://artifacthub.io/packages/helm/cert-manager/cert-manager) CRDs:

```bash
helm repo add jetstack https://charts.jetstack.io --force-update
helm install cert-manager --namespace cert-manager --version v1.16.1 jetstack/cert-manager --create-namespace --set crds.enabled=true
```

### Step 3: Install the Paladin Operator Chart

Install the Paladin operator chart:

```bash
helm upgrade --install paladin paladin/paladin-operator -n paladin --create-namespace
```

### Outcome

This process will:

1. Install the cert-manager chart.
2. Install the paladin-operator chart.
3. Create a Besu network with 3 nodes.
4. Create a Paladin network with 3 nodes, each associated with one of the Besu nodes.
5. Deploy smart contracts to the blockchain.

You can verify the running pods with:

```bash
kubectl config set-context --current --namespace paladin
kubectl get pods
```

<details>
<summary>See output</summary>

```bash
NAME                                READY   STATUS    RESTARTS      AGE
besu-node1-0                        1/1     Running   0             104s
besu-node2-0                        1/1     Running   0             104s
besu-node3-0                        1/1     Running   0             104s
paladin-node1-0                     2/2     Running   0             104s
paladin-node2-0                     2/2     Running   0             104s
paladin-node3-0                     2/2     Running   0             104s
paladin-operator-6f6864854b-bp8nb   1/1     Running   0             115s
```
</details>

Check service details with:

```bash
kubectl get service
```

<details>
<summary>See output</summary>

```bash
NAME               TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)                                                                        AGE
besu-node1         NodePort    10.96.83.214   <none>        8547:31547/TCP,30303:31627/TCP,30303:31627/UDP,8545:31545/TCP,8546:31546/TCP   6m53s
besu-node2         NodePort    10.96.73.13    <none>        8547:31647/TCP,30303:32690/TCP,30303:32690/UDP,8545:31645/TCP,8546:31646/TCP   6m53s
besu-node3         NodePort    10.96.22.234   <none>        8547:31747/TCP,30303:31724/TCP,30303:31724/UDP,8545:31745/TCP,8546:31746/TCP   6m53s
paladin-node1      NodePort    10.96.174.16   <none>        8548:31548/TCP,8549:31549/TCP,9000:31702/TCP                                   6m54s
paladin-node2      NodePort    10.96.15.53    <none>        8548:31648/TCP,8549:31649/TCP,9000:32414/TCP                                   6m54s
paladin-node3      NodePort    10.96.130.13   <none>        8548:31748/TCP,8549:31749/TCP,9000:30324/TCP                                   6m52s
paladin-operator   ClusterIP   10.96.134.58   <none>        80/TCP                                                                         7m4s
```
</details>

Check the status of the smart contract deployments with:

```bash
kubectl get scd
```

<details>
<summary>See output</summary>

```bash
NAME                                     STATUS    DEPS   TRANSACTIONID                          CONTRACT                                     TXHASH                                                               FAILURE
noto-factory                             Success   0/0    37bdf054-1586-4fbf-8fb1-8ba82e804b03   0x097e199bb09c67fa1a70f8faabd6bb6f73b46b1b   0x101d18edc452cdfb7708482a82931c008e7614ff6c827b6ecf19e603f28bd64e
pente-factory                            Success   0/0    5cb8afba-6d6b-4c69-86d1-dced6d66e72b   0x1d9490417b1aa097ea4ed5a2c7461a91a24e1b94   0x2c6ab8d2815f7ecca8f5455a279d9f318cc73812fe4e8527dfa60d3faf01fb5c
registry                                 Success   0/0    cb6c4d54-6cbb-459e-92fb-d9a4b1253008   0x8e4368f9cff103257fc0d3fee65de96da476f402   0x6a524eee8f9c3f728b8c55ef128da81658e022e01f68dd7f09a907cd0b41792b
zeto-factory                             Success   0/0    1956cd0d-57df-4ead-938d-6b2ca7c3af24   0xbe51a4d2a77dab8523062f36310cadb9491e212f   0xe47fd55373bf2c5e088c22535fb27c171db879150ca9b81743482a6dbe51b82b
zeto-g16-check-hashes-value              Success   0/0    bb89354d-d33b-4918-a76e-815204ae621f   0xce09a6f94fedbf7797aadedc607485afb3f12ae0   0xd63f0105c02473910ec7ad43d8f19907ab08c6909eefc77ece4b2815a03c74a8
zeto-g16-check-inputs-outputs            Success   0/0    e0793bf5-5daa-499e-a999-1d054b0c09cf   0x16a030887b97b7d5425159478f5545f5d37370f1   0x2f1f000f08e0189e2aa6b806d0e672dc5b6df1f1f009bf8792994d7b92b3ee18
zeto-g16-check-inputs-outputs-batch      Success   0/0    20d75131-7de9-4378-aa24-27a6788e9efe   0xacdc6af26b831a9307125e4e1ec0a5601eae6f0a   0x7dc9999281ad5d3b27408893d64ba4c7550292b64c3134d3733cbc110b6245f5
zeto-g16-verifier-anon                   Success   0/0    714cddd9-7479-4d18-8554-fd9cb49d6047   0x76da65751f561b35a7befe3c8b7f1ba1d5617882   0xc9b5fef8c99319f84a14330ebd6e4778994bdea2857a65a404920d7a2c8c838d
zeto-g16-verifier-anon-batch             Success   0/0    5037f730-cc9e-4875-8a11-428be74855ae   0x65e9b3b755d551b1a938aae02154118bfb2e7d5c   0xf1ee595b494927ca50122c9893a8acfdf93d74c6e53fd04be3b3da5f840f8a4b
zeto-g16-verifier-anon-enc               Success   0/0    906b8cd2-40c1-4992-8fe8-1e696eaf9d70   0x99287a67da14dd77d21a15536b7e3671f9a51772   0x38ae90890d20c43cfc25a05a36dd5e9b9085ba13578585b04f76c2bf7cc02ab3
zeto-g16-verifier-anon-enc-batch         Success   0/0    1b1a9713-a695-499f-a28c-c8b02f74ab82   0x1bcaff14e2c1652e2b552b0ee31c77223301b8e1   0x5f826fe4fe871afdd2214608fefda755997baffc7d110a5a1d44b81b65c9fdb9
zeto-g16-verifier-anon-nullifier         Success   0/0    e0c2a915-7b1f-43e4-a485-de2ae119e472   0x4c1f4a2d0789a74a1814101c223ce839c6f9d506   0x71ec84c3647a6d0de12fe3dcf8165cb10a06e110cdabc7d873fe5929110bb76b
zeto-g16-verifier-anon-nullifier-batch   Success   0/0    b533c381-9c68-499e-80e0-2d97a69ee530   0xf88b34f7fe5a5c5d6b3d08e555ba22560ecbb59a   0x453e5ddcb77f6462d8aad9b1e68d6b979b8d454c330f8faae10cbd1f1a7c0b1c
zeto-impl-anon                           Success   0/0    22c9cf3a-2219-4b6d-aa63-3657a480c611   0x600bfecadf35e5d88c6672dbfd9b1ed4d9e28845   0xe7e7ab84afb10e4a4086096fc4f6a96b1119b52a5560b3a90a18f34f8b8dbf93
zeto-impl-anon-enc                       Success   0/0    e39b026a-1f74-4d9d-af6b-269ccebb8dc0   0xf4cdfd7ff695500eb2a28430d59958a36d06fe54   0x854e8f34791243c9672c02d962ed23d4df6b8f6e679881961badcc430ceed855
zeto-impl-anon-nullifier                 Success   2/2    2d1d51d6-a16f-4745-bfff-9eeaeb9ae06e   0xc423ccfec92822b3354dd540fa8fa4b6b5bbd923   0x43698f6172c2be0daaa149ee5cad8c547a9ba06de1d6e0492aaf5f49516d583f
zeto-poseidon-unit2l                     Success   0/0    d40f7043-300e-4345-981d-08470514ac34   0xa74a67ed2eaa058aaa640168e7738ac9de73804e   0x8138c9ce405e56c72c6da6cac2f549a0d36b5c9443c154e49d96f1bedbbce8ca
zeto-poseidon-unit3l                     Success   0/0    f8c5de8b-dfa3-4c0e-b3b1-9cdafbb25b54   0x01b3139005de155632fb52107757b1688b937ed1   0x859a12b1705c7e061a288857d4ad9c4a27b42ca6d9fd3804a1fdaa2c11b3cc38
zeto-smt-lib                             Success   2/2    ac199436-51a4-4d91-98a3-b80b8074e3a8   0xf79adb0e771c08783d79cc223ec2be29166e8858   0x6ed5ee88c82d7f8c0ef9c56976d9f274c9a5ce5e575a9932be49b118cbdb502a
```

</details>

Check the status of Paladin nodes publishing registration details to one another:

```bash
kubectl get reg
```

<details>
<summary>See output</summary>

```bash
NAME    PUBLISHED
node1   2
node2   2
node3   2
```

</details>

## Accessing the UI

Each Paladin node runs an instance of the Paladin UI at the path `/ui`. If you used the provided kind config
(or a similar port mapping), you should be able to access the UI for each node:

* http://localhost:31548/ui
* http://localhost:31648/ui
* http://localhost:31748/ui

Learn about interacting with the [UI](./user-interface.md), and try out the [Tutorials](./../tutorials/index.md).

## Advanced installation

This guide is designed for first-time Paladin users looking for a quick and easy introduction. For more complex setups, integration with existing blockchain networks, or detailed customization, please refer to the [Advanced Installation](./installation-advanced.md) or [Manual Installation](./installation-manual.md) guides.

## Troubleshooting

If you encounter any issues during installation, please refer to the [Troubleshooting Guide](./troubleshooting.md).  

If your issue is not listed or remains unresolved, please report it by [opening an issue on the Paladin GitHub page](https://github.com/LF-Decentralized-Trust-labs/paladin/issues).

## Uninstall

To remove the Paladin operator and related resources, run the following commands:
```bash
helm uninstall paladin -n paladin
helm uninstall paladin-crds
kubectl delete namespace paladin
helm uninstall cert-manager -n cert-manager
kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.1/cert-manager.crds.yaml
kubectl delete namespace cert-manager
```
