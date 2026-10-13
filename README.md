# Paladin

Paladin brings true programmable privacy to the world's most popular smart contract platform.

![Paladin](doc-site/docs/images/paladin_overview.svg)

## Releases

Paladin currently has two release streams:

- `main` - this is where stable releases are available, with regular releases occurring at least once per month
- `v1-develop` - tracking some large upgrades that are planned as part of a V1.0 release

The primary themes for the upcoming V1.0 release are:
- Upgrades to the orchestration engine to support advanced features such as M of N endorsement policies for privacy groups
- Additional standardization of the token interfaces across notarized and ZKP backed tokens

You can track the progress towards this next project milestone here: https://github.com/orgs/LFDT-Paladin/projects/3

Keep an eye on discussion on the [paladin-maintainers channel in Discord](https://discord.com/channels/905194001349627914/1332404027052392488) for updates on progress with the release and discussion of any key features being worked. If you have questions about the release and timings please drop questions in that channel.

## Documentation

Read the official [documentation](https://LFDT-Paladin.github.io/paladin/head)
for more details on Paladin.

Join the discussion in the "paladin" channel on the
Linux Foundation Decentralized Trust [Discord server](https://discord.com/channels/905194001349627914/1303371167020879903).

![Paladin](doc-site/docs/images/paladin_runtime.svg)

## Getting started with Paladin

Get a 3-node Paladin network running with Besu on your laptop with the
[Getting Started](https://LFDT-Paladin.github.io/paladin/head/getting-started/installation)
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


> The build is optimized for computers with **16GB RAM** by default. For slower computers:
>
> **Quick Setup:**
>  ```bash
>  # Run with optimized settings
> ./gradlew build --max-workers=2 --parallel
> ./gradlew build --max-workers=1       # For very slow laptops
>  ```

### Running a full development environment

Check out the operator readme:

- [operator/README.md](operator/README.md)

### Contributing

We welcome community contributions! Please review the [guidelines](https://www.lfdecentralizedtrust.org/how-to-contribute)
for contributing to Linux Foundation Decentralized Trust projects.

If you're looking for somewhere to help, you can begin by looking through
[open issues](https://github.com/LFDT-Paladin/paladin/issues), or join
the conversation on [Discord](https://discord.com/channels/905194001349627914/1303371167020879903).
