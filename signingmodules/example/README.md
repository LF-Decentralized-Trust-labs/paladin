# Example signing module

Go signing module that provides a simple signing module plugin using a configurable in-memory signer.

## Overview

This `example` signing module provides a complete signing module plugin in Paladin.

## Using the signing module

### Build the signing module in a Paladin image

To use the `example` as a signing module plugin (or when adding your own), you will need to include it in the Paladin build:

In the root `build.gradle` file of the Paladin project:

- Add it to the list of subprojects to assemble in the `assembleSubprojects` def list:

```
def assembleSubprojects = [
  ...
  ':signingmodules:example',
  ...
  ]
```

- Add the signing module lib location to the `signingmodules` def list to ensure that any library plugin is copied to the build output:

```
def signingmodules = [
    'signingmodules/example/build/libs',
]
```

Add it to the built output in the builder phase for the Paladin `Dockerfile`:

```
FROM base-builder AS full-builder
...
COPY signingmodules/example signingmodules/example
...
```

This will then include the built signing module `.so` file in the Paladin image, for example when you run `gradle buildPaladinImage` from the `/operator` directory.

### Configuring the signing module

A signing module plugin will typically require configuration as per any plugin in the Paladin ecosystem. For a signing module this is achieved by setting the appropriate configuration in the `signingModules` section in the configuration for the Paladin node.

```
"signingModules": {
  "<plugin-name>": {
    "plugin": {
      "type": "<plugin-type>",
      "library": "<plugin-location>"
    },
    "config": <plugin-config>
  }
},
```

For the `example` signing module, it will take a configuration object with a `signer` value that is of the Paladin `SignerConfig` type. Below is a signer that is configured as a static key store, with an initial seed phrase that is set to perform BIP32 key derivation:

```
"signingModules": {
  "example": {
    "plugin": {
      "type": "c-shared",
      "library": "/app/signingmodules/libexample.so"
    },
    "config": {
      "signer": {
        "keyStore": {
          "type": "static",
          "static": {
            "keys": {
              "seed": {
                "encoding": "none",
                "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"
              }
            }
          }
        },
        "keyDerivation": {
          "type": "bip32"
        }
      }
    }
  }
}
```

Alternatively a signing module plugin can also be defined as part of a Paladin custom resource when managed by the Paladin operator, by setting the appropriate configuration in the `signingModules` section in the `spec` for the Paladin custom resource:

```
signingModules:
  - name: <plugin-name>
    plugin:
      type: <plugin-type>
      library: <plugin-location>
    configJSON: |
      <plugin-config>
```

To then use the signing module a wallet must be configured that references it, by setting the appropriate configuration in the `wallets` section in the configuration for the Paladin node, for example:

```
"wallets": [
  {
    "name": "example-signer-1",
    "keySelector": ".*",
    "keySelectorMustNotMatch": false,
    "signerType": "plugin",
    "signerPluginName": "example"
  }
]
```

Please see https://lf-decentralized-trust-labs.github.io/paladin/head/architecture/key_management/ for more information on key management and signing modules.
