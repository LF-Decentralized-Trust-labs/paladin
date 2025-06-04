# Key Management

> TODO: Well established architecture already being brought across to Paladin here (Lead: Matt Clarke)

![Key Management](../images/key_management.jpg)

Key management in Paladin is designed to meet a complex set of requirements for enterprise
key management, spanning both the secure storage of key materials in advanced locations (HMS/SSM),
and dynamic usage of keys for many different identities/wallets/one-time use cases.

Paladin has the extra complexity over many Web3 technologies, that it doesn't just need to support
a single algorithm (such as SECP256K1). Instead it must support multiple privacy-perserving
approaches to signing - including the use of key materials directly during the formation of
Zero Knowledge Proofs (ZKP).

## Choices for the key administrator

This leads to some key decisions that must be made by the administrator responsible for the 
Paladin node on how keys are managed.

### In-memory vs in-key-store signing

Advanced cryptographic storage systems such as Hardware Security Modules (HSM) and Software
Security Modules (SSM) / Vault technologies, are built on one very important principal:

_The key materials never leave the store_

There are two challenges to this in the Web3 domain:

1. **Algorithm / ZKP support** - HSM/SSM modules may not support all of the algorithms required
for signing, or the ability to execute ZKP circuit provers natively.
2. **Numeracy of keys** - HSM/SSM modules are commonly optimized for high value keys, and
thus generating/storing millions of single-use keys for anonymous TX transaction submission 
might be inefficient (on cost or performance)

Paladin lets you choose _use case by use case_ all within a single Paladin engine whether you
delegate signing into your key store using its internal cryptography, or whether you allow the
signing key to be loaded into the volatile memory of the `signing module` to perform the
cryptography.

### Paladin-embedded vs. remote signing modules

Because there are going to be cases where you perform in-memory signing, or proof generation,
you should be cautious about where the signing modules run that will hold those keys
in volatile memory.

- You might choose to have that happen embedded into the Paladin node
    - The shortest code path for performance
    - The simplest deployment architecture
- You might choose to run them on a completely separate infrastructure
    - In a more trusted network segment, with very limited secure key path for signing requests 
    - Maybe co-located with your HSM with a local PKCS#11 interface
    - Maybe runtime-embedded into your SSM / Vault technology as a code-module

Paladin packages the signing module for maximum flexibility. You will see all the quick start
setup guides, and kubernetes deployment samples, have it Paladin-embedded by default.

However, the code is structured to make it very easy to run it remotely.

You can extend it with code to support more key storage technologies, including proprietary
technologies unique to your enterprise. The modular code design for extensibility, is combined
with a set of options on remote connectivity:
- HTTPS+JSON
- gRPC+Protobuf
- Both with mutual-TLS and additional session/JWT credentials

> Multiple signing-modules are supported by a single Paladin node, so you can use a mixture
> of embedded and remote signing modules in one node

### Direct key mapping vs. key derivation 

We discussed earlier that the numeracy of keys used by Web3 technologies (particularly with
anonymity), and speed of key generation, is a challenge for some HSM/SSM technologies.

So for each use case you need to make an important choice between:

#### Direct key mapping

Here there is a 1:1 relationship between a resolved `key mapping` for a key stored in the Paladin
database, and a record of a key (individual piece of cryptographic material) in
your key storage technology.

Paladin makes every effort to ensure this mapping is _bidirectional_.

The identifier/name/label for your keys that make them unique within your key storage
is bound to exactly one key mapping in your Paladin database that applications/users can use
to reference that key.

This means that either of the following result in the same outcome:

1. An application requests usage of a key, which causes Paladin to `resolve` it in the backing
   key storage system. A piece of key material is _looked up or created_ in the key storage system
   and the `key mapping` is stored in Paladin.
2. A set of existing keys have been created by an administrator in the key storage system,
   and Paladin is instructed to `discover` all of these keys. Multiple `key mappings` are created
   in the Paladin database, each referring to a separate key in the key store.

> Both in-memory and in-key-store signing are possible with direct key mapping, depending
> on the capabilities of the backing key storage HSM/SSM.

#### Key derivation (BIP32)

Here there is a many:1 relationship between a resolved `key mapping` for a key, and a
`seed` piece of key material looked up or created at startup in the key storage system.

This single seed is used to build a near-infinite supply of unique keys, structured
in a hierarchy of parent/child keys that can be deterministically retrieved in a very
efficient manner.

This is commonly referred to as a Hierarchical Deterministic (HD) wallet.

> Only in-memory signing is possible when key derivation is performed within the signing
> module, as only the `seed` is stored inside the HSM/SSM key storage module.

Paladin supports a well established set of standards for the operation of this
key derivation:
- [BIP-32](https://en.bitcoin.it/wiki/BIP_0032) defines the fundamental operation of the
derivation algorithm for cryptographic keys
- [BIP-39](https://en.bitcoin.it/wiki/BIP_0039) allows mnemonic seed phrases to _optionally_
be used (instead of a 32 byte private key) as the `seed`, or root, of the key hierarchy
- [BIP-44](https://en.bitcoin.it/wiki/BIP_0044) provides a string semantic for expressing
a derivation path within a BIP-32 HD Wallet with a string pattern such as
`m / 44' / 60' / 0' / 1 / 2 / 3`. This syntax is used in the signing module configuration
for the prefix to use for keys, and as the way to refer uniquely to a key in the hierarchy.

Some key storage systems internally use key derivation, similarly to that performed by
the signing module. In these cases the signing module is configured for `direct` key mapping,
and any key derivation is delegated down into the backing key store.

#### Mixing direct and derived key mapping

You can use multiple signing modules in a single Paladin, including differently configured
modules backed by the same key store. So you can use a mix of direct mapping and key derivation
in a single Paladin node.

## How it works for applications

The architecture has the following core concepts:

### 1. Key Identifiers

When applications and configuration refer to keys, they can do so via string identifiers.

> See [Data & Registry](./data_and_registry.md) for details about the format of these identifiers
> and how they are resolved across separate Paladin runtimes. 

These identifiers can be human/application friendly strings describing the **purpose** of the key,
rather than needing to be one of the public-key identifiers (like an Eth `address`) that
represents that key with a particular signing algorithm (like `secp256k1`).

Key identifiers can be organized into folders, using a `/` character within the identifier.

This is useful for logical partitioning of keys owned by different entities, as well as being
used by the signing module to influence grouping of keys. For example to influence the derivation
path of keys in a BIP32 Hierarchical Deterministic (HD) Wallet.

When submitting public transactions, the key may also be referred to using the Ethereum Address
(Referred to in Paladin as a `Verifier`, described in detail in the
[Public Verifiers and Algorithms](#3-public-identifiers-and-algorithms) section below). To do this, the `from`
string of a transaction must be prefixed with `eth_address:`, e.g.
`"from": "eth_address:0xf8f3fcf26a437cac6f8bdc92257f3b03e2f5c546`. The syntax is case sensitive, and
Etherum Addresses are stored in lower case. Referring to keys by public verifier is not supported more widely
as cross node reverse lookups may result in identifiers being leaked.

> Note that resolving a key in a backend key storage system by verifier (derived from the public key) is only possible in Paladin, after that verifier has been resolved at least once for the correct `algorithm` and `verifierType`. Otherwise Paladin does not have a reverse-lookup mapping stored in the database to be able to perform the resolution.

### 2. Key Mappings

These are database persisted, and cached, records that the main Paladin runtime maintains that match
a `key identifier` to the `key handle` of that key in the signing module.

Key mappings have a reference to the folder they are in, which references its parent folder
all the way up to a single root folder that is pre-created by the Paladin runtime.

Key mappings also have `attributes` that can be specified when creating a key mapping explicitly
over an API, and are passed to the signing module when resolving the key. This allows the behavior of
the signing module when obtaining/creating key materials to be customized at runtime (within the 
constraints of that signing module).

Every `key mapping` and `folder` gets two attributes automatically:
- `name`: the part of the `key identifier` representing this key / folder
- `index`: a numeric identifier, assured to be unique at this folder level

### 3. Public key identifiers (or "verifiers") and algorithms

Cryptographic keys use public/private cryptography, meaning every key can be identified publicly in a way that does not leak the key information itself.

Any party can _verify_ that a transaction was signed with a particular private key, by _recovering_ the public key used to sign that transaction.

However, to do this we need a standard way to represent a public key so that it can be verified in a standard way against a transaction. Each cryptographic ecosystem actually does this differently, even when using the same private key, and the same _algorithm_ (such as SECP256K1).

The most obvious example of this is the Ethereum address. This is a 20 byte compressed representation of a SECP256K1 public key, derived using a well documented algorithm. It looks like `0xfdcba455d748cb3e085472cc6f49b4ae86ee4d1f` in hex, and sometimes is represented in a case-sensitive way to provide a checksum and avoid copy/paste errors like `0xFdcBa455D748cB3e085472CC6F49B4AE86eE4d1F` per the EIP-55 standard.

Paladin supports multiple of these "verifiers" to be calculated, and stored, for public keys. This is fully pluggable, so all the different types of cryptography used in Paladin.

For example the IDEN3 standards for representing public keys with Baby JubJub are plugged in by the Zeto domain that implements signing proofs within zero-knowledge proof (ZKP) circuits.

In Paladin transaction signing can be complex, requiring multiple signatures, using
different algorithms, at different stages in the assembly, endorsement/proof and
submission of the transaction to the base blockchain.

An algorithm might be straight forward signing of a payload, such as `secp256k1`
signing, and maybe can happen natively inside a HSM device.

An algorithm might be domain specific, such as usage of a ZKP friendly cryptography
inside of the proof generator of a specific circuit generated in a toolkit like Circom.

Some of these algorithms have different ways to represent the same key materials,
and require distribution of those public verifiers to multiple parties during
the creation of endorsements/proofs of the transaction.

So Paladin has a scheme for identification of the `algorithm` for a signing/proof
generation request, and associating multiple `public verifiers` to the same
key materials.

### 4. Signing Modules

These are the engines that have direct or indirect access to key materials, and coordinate
signing and ZKP proof generation.

> Note that the key materials themselves are stored outside the signing module,
> in one of a number of places supported by those signing modules - such as
> Local disk (for dev), Cloud Key Managers, Cloud HSMs, Software key managers,
> and HSMs (supporting PKCS#11)

Paladin embeds a flexible implementation of the signing module into the runtime
with convenient access to key materials and signing algorithms.

Paladin provides this same code as a base implementation for running signing modules
outside the core runtime connected of a mutual-TLS secure connection, supplemented
with JWT credentials for each signing request that propagate context from the Paladin
runtime on the context of the signing.

### 5. Key resolution and creation of new keys on-demand

The resolution from a `key identifier` to a `key handle` happens dynamically, when a
suitable `key mapping` is not already found in the Paladin database.

The result of this resolution always ends up with a `key handle` that the signing module
produces that uniquely identifies that key inside of the signing module's cryptographic
storage (which is pluggable to many technologies).

However, the resolution result might end up with many possible outcomes.

For example:
- Returning the identifier of a key that already was pre-created in the cryptographic
  storage system, thus establishing a new `key mapping` to an existing `key handle`
- Instructing the cryptographic storage system to generate a brand new key, thus 
  on-demand creating a `key mapping` to a new unique key
- Allocating a unique derivation path in a Hierarchical Deterministic (HD) derivation
  path scheme like BIP32, that references a new unique key backed by an existing
  seed/mnemonic stored in the cryptographic storage system