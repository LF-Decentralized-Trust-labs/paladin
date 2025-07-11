# Workflow Overview

This repository is equipped with automated workflows that streamline key processes for PRs, changes to the `main` branch, and releases. These workflows ensure smooth development, testing, and deployment cycles.

## PR Opened 🚦
When a developer opens a PR, several automated checks are triggered to validate the changes:

- **[Build (PR)](workflows/on-pr.yaml):**  
  Runs essential tasks to ensure code quality and reliability:
  - **Build and Test:** Compiles the code and runs tests for all subdirectories using [build.yaml](workflows/build.yaml).
  - **[Build Docker Images](workflows/build-image.yaml):** Builds Docker images based on PR changes for local validation.  
    > **Note:** These images are **not published** to a registry.
  - **[Template the Helm Chart](workflows/build-chart.yaml):** Rebuilds and validates Helm charts for correctness.  
    > **Note:** Charts are **not published** but tested locally.

All checks must pass before PRs can be merged to the main branch.


## Changes Pushed to Main 🌟
Once changes are merged into the `main` branch, workflows prepare the project for production:

- **[Build (push)](workflows/on-push.yml):**  
  Similar to PR checks, this ensures the integrity of the main branch:
  - **Build and Test:** Compiles code and runs tests for all subdirectories using [build.yaml](workflows/build.yaml).

- **[Publish Docker Images](workflows/cross-build-images.yaml):**  
  Produces production-grade, cross-platform Docker images and publishes them to the container registry:
  - **Registry:** `ghcr.io/lf-decentralized-trust-labs`
  - **Images:** `paladin`, `paladin-operator`
  - **Tagging:** Images are tagged with `main`.
  - **Platforms:** `linux/amd64`, `linux/arm64`

- **[Update Documentation](workflows/docs.yaml):**  
  Detects documentation updates and publishes the latest content to the documentation site.


## Release Time 🚀
Releases deliver artifacts and resources to users and deployment targets through these workflows:

- **[Release Orchestrator](workflows/release.yaml):**  
  Triggered by a version tag (e.g., `v1.2.3`), this workflow coordinates all release activities:
  - **[Release Docker Images](workflows/release-images.yaml):**  
    Builds and **publishes Docker images** tagged with the release version (e.g., `v1.2.3`) and `latest`.
    - **Registries:** 
      - `ghcr.io/lf-decentralized-trust-labs`
      - `docker.io/lfdecentralizedtrust`
    - **Images:** `paladin`, `paladin-operator`
    > `latest` is configurable 
  - **[Release Helm Chart](workflows/release-charts.yaml):**
    Packages and **publishes Helm charts** to the chart repository tagged with the release version (e.g., `v1.2.3`) and `latest`.
    > **Includes E2E Testing:** This workflow automatically runs comprehensive end-to-end tests before publishing
    > `latest` is configurable 
  - **[Release TypeScript SDK](workflows/release-typescript-sdk.yaml):**  
    Updates and **publishes the TypeScript SDK** to npm:
    - **Package:** `@lfdecentralizedtrust/paladin-sdk`
    - **Registry:** [npm](https://www.npmjs.com/package/@lfdecentralizedtrust/paladin-sdk)
  - **[Release Solidity Contracts](workflows/release-solidity-contracts.yaml):**
    Packages contract ABIs and deployment artifacts for distribution.

### Releasing Options: 
1. **Automatic:** Push a Git tag in the format `vX.Y.Z` (e.g., `v1.2.3`), and the workflows handle the release, marking it as the latest.
2. **Manual:** Trigger the [release workflow](https://github.com/LF-Decentralized-Trust-labs/paladin/actions/workflows/release.yaml) via the GitHub Actions interface, specifying the version and selecting the "latest" option if needed.


## Manual Actions 🛠️
Workflows can also be triggered manually when needed. Available options include:

- **[Release Orchestrator](workflows/release.yaml)**
- **[Release Docker Images](workflows/release-images.yaml)**
- **[Release Helm Chart](workflows/release-charts.yaml)**
- **[Release TypeScript SDK](workflows/release-typescript-sdk.yaml)**
- **[Release Solidity Contracts](workflows/release-solidity-contracts.yaml)**
- **[Build Helm Chart](workflows/build-chart.yaml)**
- **[Build Docker Images](workflows/build-image.yaml)**  
- **[Cross-Platform Docker Image Build](workflows/cross-build-images.yaml)**
- **[Build Project](workflows/build.yaml)**
- **[Build (PR)](workflows/on-pr.yaml)**
- **[Build (push)](workflows/on-push.yml)**
- **[Docs Site](workflows/docs.yaml)**

## Additional Workflows

- **[Stale Issues/PRs](workflows/stale.yml):** Automatically marks and closes stale issues and pull requests
- **[Build Workflows](workflows/build-workflows.yaml):** Validates workflow syntax and structure 