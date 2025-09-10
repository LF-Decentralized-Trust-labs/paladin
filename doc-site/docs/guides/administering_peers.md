# Administering peers

<!-- TODO: this section could include an overview of our reference registry implementations. The architecture page needs to be filled in for the generic registry architecture. -->

<!-- TODO: talk about the difference in behaviour between directCertVerification being enabled or not -->

## Debugging peering issues in the reference `evm` registry

Errors such as: 
```
PD012100: No entries found for node 'node2'
```
```
"PD011206: TRANSPORT grpc returned error: PD030015: GRPC connection failed for endpoint 'dns:///paladin-node2.paladin.svc.cluster.local:9000': rpc error: code = Unavailable desc = connection error: desc = \"transport: authentication handshake failed: PD030007: peer 'node2' did not provide a certificate signed an expected issuer received=CN=node2 issuers=[CN=node1]: x509: certificate signed by unknown authority\""
```
can occur when

- a node hasn't been published to the registry
- a node has been published to the registry but the events emitted from its registration have not been indexed
- there is a problem with the transport details published to the registry

If the error you are seeing is a certificate error, you can go directly to step 3 of this troubleshooting section.

> N.B. The steps below may ask you to make RPC requests to the "registry admin node". This is `node1` for `devnet` installations or the first node in the `paladinNodes` array from the values file for `customnet` installations.

### 1. Confirm the status of the `paladinregistration`

Run
``` 
kubectl get paladinregistration
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
If the output shows 2 publications per node then registrations have completed successfully. 

If any of the numbers in the published column are below 2, get the detailed status for that node
```
kubectl describe paladinregistration node1
```
<details>
<summary>See output</summary>
```bash
Name:         node1
Namespace:    default
Labels:       app.kubernetes.io/managed-by=Helm
              app.kubernetes.io/name=operator-go
Annotations:  meta.helm.sh/release-name: paladin
              meta.helm.sh/release-namespace: default
API Version:  core.paladin.io/v1alpha1
Kind:         PaladinRegistration
Metadata:
  Creation Timestamp:  2025-09-09T15:11:15Z
  Generation:          1
  Resource Version:    2350
  UID:                 6800bc37-f317-4b51-9799-872ad8f2aeff
Spec:
  Node:                 node1
  Node Admin Key:       registry.node1
  Registry:             evm-registry
  Registry Admin Key:   registry.operator
  Registry Admin Node:  node1
  Transports:
    grpc
Status:
  Publish Count:  1
  Publish Txs:
    Grpc:
      Idempotency Key:     k8s.reg.node1.grpc.1757430744774547
      Transaction ID:      83278a5f-5f89-412c-b8a4-9757bdb1f382
      Transaction Status:  Pending
  Registration Tx:
    Block Number:        6
    Idempotency Key:     k8s.reg.node1.1757430743709513
    Transaction Hash:    0xaa92ab37b2afab75dd77d04e0039e5e0748777b5d0ca72591c9ceb9d9907deea
    Transaction ID:      8cdae679-a0ce-43e4-9be0-62a4313d37dc
    Transaction Status:  Success
Events:                  <none>
```
</details>
If one of the transactions has `Submitting` status, it has not yet been submitted to a Paladin node. Further information may be available in the operator logs.

If one of the transactions has `Pending` status, there should also be a `Transaction ID` available. This transaction ID can be used to debug the transaction with a `ptx_getTransactionFull` RPC call. This call needs to be submitted to:

- the registry admin node in order to debug the `Registration Tx` transaction
- the node being registered in order to debug the `Publish Txs.Grpc` transaction.
<details>
<summary>See example curl command</summary>
```bash
curl http://localhost:31548 --header 'Content-Type: application/json' \
--data '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "ptx_getTransactionFull",
    "params": ["83278a5f-5f89-412c-b8a4-9757bdb1f382"]
}'
```
</details>
<!-- TODO: There's scope here to write a whole guide on debugging pending transactions- e.g. indexer not yet caught up, insufficient funds etc. -->

### 2. Confirm that all registration events have been indexed

This can be confirmed in the `Registry` tab of the Paladin UI or by making a `reg_queryEntriesWithProps` RPC call.
<details>
<summary>See example curl command</summary>
```bash
curl http://localhost:31548 --header 'Content-Type: application/json' \
--data '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "reg_queryEntriesWithProps",
    "params": ["evm-registry", {"limit": 100}, "any"]
}'
```
</details>

There should be an entry for every node, and each node entry should have a `transport.grpc` property. This check needs to be made against every node in the installation, as they independently build their registries by indexing chain events.

> N.B. `root` is the root identity of the registry. It does not correspond to a node and is not expected to have any properties.

If any node or property is missing it means that the event which held that information has not been indexed. This is usually due to the block indexer being configured to start from a later block than the start of the registry history, or the block indexer having not yet reached the block that contains the relevant event.

First identify the block number which contains the event with the missing data by looking at the detailed registration status for the node which is missing or has the `transport.grpc` property missing.
```
kubectl describe paladinregistration node1
```
<details>
<summary>See output</summary>
```bash
Name:         node1
Namespace:    default
Labels:       app.kubernetes.io/managed-by=Helm
              app.kubernetes.io/name=operator-go
Annotations:  meta.helm.sh/release-name: paladin
              meta.helm.sh/release-namespace: default
API Version:  core.paladin.io/v1alpha1
Kind:         PaladinRegistration
Metadata:
  Creation Timestamp:  2025-09-09T15:11:15Z
  Generation:          1
  Resource Version:    2350
  UID:                 6800bc37-f317-4b51-9799-872ad8f2aeff
Spec:
  Node:                 node1
  Node Admin Key:       registry.node1
  Registry:             evm-registry
  Registry Admin Key:   registry.operator
  Registry Admin Node:  node1
  Transports:
    grpc
Status:
  Publish Count:  2
  Publish Txs:
    Grpc:
      Block Number:        11
      Idempotency Key:     k8s.reg.node1.grpc.1757430744774547
      Transaction Hash:    0x6f02b322be187569922ec8f2143e3f8ad5ce39e1eae874d0e2191c17ffc1f5a6
      Transaction ID:      83278a5f-5f89-412c-b8a4-9757bdb1f382
      Transaction Status:  Success
  Registration Tx:
    Block Number:        6
    Idempotency Key:     k8s.reg.node1.1757430743709513
    Transaction Hash:    0xaa92ab37b2afab75dd77d04e0039e5e0748777b5d0ca72591c9ceb9d9907deea
    Transaction ID:      8cdae679-a0ce-43e4-9be0-62a4313d37dc
    Transaction Status:  Success
Events:                  <none>
```
</details>
- if the node is missing look at the block number of the `Registration Tx` transaction
- if the `transport.grpc` property is missing look at the block number of the `Publish Txs.Grpc` transaction.

Now we have a block number, we can check that the block indexer has indexed it by making two `bidx_queryIndexedBlocks` to get the first and last blocks that the node has indexed. Make these calls against the node which is missing data in its registry.
<details>
<summary>See example curl command for getting the first indexed block</summary>
```bash
curl http://localhost:31548 --header 'Content-Type: application/json' \
--data '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "bidx_queryIndexedBlocks",
    "params": [{"limit":1, "sort": ["number ASC"]}]
}'
```
</details>
<details>
<summary>See example curl command for getting the last indexed block</summary>
```bash
curl http://localhost:31548 --header 'Content-Type: application/json' \
--data '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "bidx_queryIndexedBlocks",
    "params": [{"limit":1, "sort": ["number DESC"]}]
}'
```
</details>

If the block number is within the indexed range, look at the Paladin node pod logs to see if there are any errors writing registry data to the database.

If the block number is above the indexed range, the node likely just needs more time to index all the blocks it needs for the registry. If the last indexed block is not increasing over time, look at the Paladin node pod logs for indexing errors.

If the block number is below the indexed range, you are likely using a `customnet` installation and have overriden the default config to set a custom `fromBlock` for the block indexer.

<details>
<summary>See example of overriding the default block indexer from block</summary>
```yaml
paladinNodes:
  - name: node1
    config: |
      blockIndexer:
        fromBlock: 17543 # can also be latest
```
</details>

If any of the nodes have started indexing from a later block than one which contains an event they require, it is difficult to recover the situation. It is typically easier to recreate the system, ensuring `fromBlock` is set to a block number that will come before the next registry contract deployment, than to take any sort of remedial action.

### 3. Confirm registry contents

If all nodes have a fully populated registry, the next step is to verify the contents of the registry.

The transport properties for a node can be found in the `Registry` tab of the Paladin UI or by making a `reg_queryEntriesWithProps` RPC call.
<details>
<summary>See example curl command</summary>
```bash
curl http://localhost:31548 --header 'Content-Type: application/json' \
--data '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "reg_queryEntriesWithProps",
    "params": ["evm-registry", {"limit": 100}, "any"]
}'
```
</details>
<details>
<summary>See example properties</summary>
```json
{
   "$owner": "0xbd788c85660d249f23e084ee25cf82dd63861965",
   "transport.grpc": "{\"endpoint\":\"dns:///paladin-node3.default.svc.cluster.local:9000\",\"issuers\":\"-----BEGIN CERTIFICATE-----\\nMIIC/jCCAeagAwIBAgIQTFQ97yE+BI1FVWT53PlyHDANBgkqhkiG9w0BAQsFADAQ\\nMQ4wDAYDVQQDEwVub2RlMzAeFw0yNTA5MDkxNTExMThaFw0yNTEyMDgxNTExMTha\\nMBAxDjAMBgNVBAMTBW5vZGUzMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC\\nAQEAwhcU9dX0jAqIymu23QwIwBkQsh/Tj9AOUNIlVlAL7+Wrx/7bKoeUCW9kU4hn\\nGJSC23NGAtPWckXAVlTyWzz+Ssn8UCEUO/9drwhJPjXHjKmnwSI2is0koSG0Rm9e\\nFBfpi56KqbByXgDaiTzJDeANr67hKnB5ve6KlolTFheQdwiJ/rKI7TOlX8VgKHlj\\nPpIVxdpbmkwtOHJMl039RG6hWuCaf3r5AOreNKU2XxdFK51Zn0Md4KdcMs9HYOKj\\n0LSz8pw4rM8YJsb5RRcAtLljf0QRzjIrPF+70iWvZY0cvQiKbRr/J6fYN3Door+I\\nVJW8DD4h0jA3EOOk9deFO7WKTQIDAQABo1QwUjAOBgNVHQ8BAf8EBAMCBaAwDAYD\\nVR0TAQH/BAIwADAyBgNVHREEKzApgidwYWxhZGluLW5vZGUzLmRlZmF1bHQuc3Zj\\nLmNsdXN0ZXIubG9jYWwwDQYJKoZIhvcNAQELBQADggEBAHKHxoLYfd0VrzD0NXL9\\nJnnPHxF9S5u6uhMtJtFXtgOixQbTzM4eNL6Z+HtpJRbEZpUGeFvb85gmPryY2OvB\\nzlNZWPnkpfmrQnmFZOlmcjWh/slK8mp7DUjQMeYL42YevLmv2sn+eEbKNO7Z3rDw\\nX8IdOCJju5+HUeGnDobkgCWT8fx9xu6+RLIOhpsUQocoDSSTDP1b3kOjs8vQbF/T\\nCs1g3tP7tE4ARZOlSJXqacCkeStpXftpkI2vAm2bDf3dTNsHxjxRHU1GnEQQFyMW\\nXjvxzuL5J8ArAUVyfyhYjaC0AFMkbaedXI+MsCOmROGYdfYQWZyNbB7HNSJJny4D\\n7gI=\\n-----END CERTIFICATE-----\\n\"}"
}
```
</details>

#### Incorrect endpoint: `customnet` only
`endpoint` in the `transport.grpc` properties is set to the internal cluster host name by default, which allows nodes installed into the same Kubernetes cluster to communicate with each other, but can be overriden in a `customnet` installation in the `transports` section of the values file for each node. If you have set this value, including setting it to the empty string `""`, confirm that the dns lookup of this hostname will work correctly for your infrastructure. 

```yaml
transports:
   - name: grpc
   config:
      port: 9000
      address: 0.0.0.0
      externalHostname: "..." 
```

If you need to update the `externalHostname` with a helm upgrade, follow [these instructions](./certificate_management.md#update-the-registry) to make the corresponding update to the registry. The instructions talk about certificate rotation but since the endpoint is included in the local transport details that get updated in the registry, they are applicable here as well.

#### Overly restrictive network policies
Check that you do not have any network policies which prohibit nodes from communicating with each other on the grpc port.

#### Invalid certificates: `customnet` only
`customnet` installations allow more flexibility in how certificates are configured. The values file specifies a secret name which

- if it exists will be mounted into the Paladin node pod as is (this option allows for use of a [custom certificate manager](./certificate_management.md#custom-certificate-manager))
- if it does not exist will be used by the Paladin operator to store `cert-manager` certificates

```yaml
transports:
   - name: grpc
      tls:
         secretName: paladin-bank-mtls  
         certName: paladin-bank-mtls
```

Ensure that all the `secretName` and `certName` (if using the Paladin operator to generate certificates) are unique per Paladin node. Duplication can result in one node using the certificates generated for another node.

If using a custom certificate manager, inspect the certificates in this secret specified by `secretName` to confirm that:

- they are valid
- they have not expired
- the common name is the node name
- the published node endpoint is a subject alternative name

If you make any changes to the certificates that each node is using, you will need to make a corresponding update to the registry by following [these instructions](./certificate_management.md#update-the-registry).

#### Rotated certificates

A certificate manager (including `cert-manager` which the Paladin operator uses by default) may rotate certificates automatically. In this case the Paladin node pod will automatically pick up the changes, but they will not be automatically reflected in the registry. To identify whether there is a discrepency between the certificates that the Paladin node is using and what is published in the registry, compare the `transport.grpc` property in the registry to the result of a `transport_localTransportDetails` call.

<details>
<summary>See example curl command</summary>
```bash
curl http://localhost:31548 --header 'Content-Type: application/json' \
--data '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "transport_localTransportDetails",
    "params": ["grpc"]
}'
```
</details>

If they are different, follow [these instructions](./certificate_management.md#update-the-registry) for updating the registry.
