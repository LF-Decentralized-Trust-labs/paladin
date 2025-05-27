# Paladin

Paladin brings true programmable privacy to the world's most popular smart contract platform.

![Paladin](doc-site/docs/images/paladin_overview.svg)

## Documentation

Read the official [documentation](https://lf-decentralized-trust-labs.github.io/paladin/head)
for more details on Paladin.

Join the discussion in the "paladin" channel on the
Linux Foundation Decentralized Trust [Discord server](https://discord.com/channels/905194001349627914/1303371167020879903).

![Paladin](doc-site/docs/images/paladin_runtime.svg)

## Getting started with Paladin

Get a 3-node Paladin network running with Besu on your laptop with the
[Getting Started](https://lf-decentralized-trust-labs.github.io/paladin/head/getting-started/installation)
guide.

![Paladin](doc-site/docs/images/paladin_deployment.svg)

## Development

### Building locally

Install the following pre-requisites:

- JDK - <https://adoptium.net/download/>, requires version 17 or above
- Protoc - <https://grpc.io/docs/protoc-installation/>
- Docker - <https://docs.docker.com/compose/install/>
- npm/NodeJS - <https://nodejs.org/en/download/package-manager>
- Go - https://go.dev/doc/install

Then run the following command to build Paladin via [Gradle](https://gradle.org/):

```shell
./gradlew build
```

### Running a full development environment

Check out the operator readme:

- [operator/README.md](operator/README.md)

### Contributing

We welcome community contributions! Please review the [guidelines](https://www.lfdecentralizedtrust.org/how-to-contribute)
for contributing to Linux Foundation Decentralized Trust projects.

If you're looking for somewhere to help, you can begin by looking through
[open issues](https://github.com/LF-Decentralized-Trust-labs/paladin/issues), or join
the conversation on [Discord](https://discord.com/channels/905194001349627914/1303371167020879903).
