# Workflow Overview

This repository is equipped with automated workflows that streamline key processes for PRs, changes to the `main` branch, and releases. These workflows ensure smooth development, testing, and deployment cycles.

## PR Opened 🚦
When a developer opens a PR, several automated checks are triggered to validate the changes:

- **[Build the Project](workflows/paladin-PR-build.yml):**  
  Runs essential tasks to ensure code quality and reliability:
  - **Build and Test:** Compiles the code and runs tests for all subdirectories.
  - **[Build Docker Images](workflows/build-image.yaml):** Builds Docker images based on PR changes for local validation.  
    > **Note:** These images are **not published** to a registry.
  - **[Template the Helm Chart](workflows/build-chart.yaml):** Rebuilds and validates Helm charts for correctness.  
    > **Note:** Charts are **not published** but tested locally.


## Changes Pushed to Main 🌟
Once changes are merged into the `main` branch, workflows prepare the project for production:

- **[Build the Project](workflows/paladin-PR-build.yml):**  
  Similar to PR checks, this ensures the integrity of the main branch:
  - **Build and Test:** Compiles code and runs tests for all subdirectories.

- **[Publish Docker Images](workflows/cross-build-images.yaml):**  
  Produces production-grade, cross-platform Docker images and publishes them to the container registry:
  - **Registry:** `ghcr.io/<repository-owner>`
  - **Tagging:** Images are tagged with `main`.

- **[Update Documentation](workflows/docs.yaml):**  
  Detects documentation updates and publishes the latest content to the documentation site.


## Release Time 🚀
Releases deliver artifacts and resources to users and deployment targets through these workflows:

- **[Release Orchestrator](workflows/release.yaml):**  
  Triggered by a version tag (e.g., `v1.2.3`), this workflow coordinates all release activities:
  - **[Release Docker Images](workflows/release-images.yaml):**  
    Builds and **publishes Docker images** tagged with the release version (e.g., `v1.2.3`) and `latest`.
    - **Registries:** 
      - `ghcr.io/<repository-owner>`.
      - `docker.io/<repository-owner>`.
    > `latest` is configurable 
  - **[Release Helm Chart](workflows/release-charts.yaml):**
    Packages and **publishes Helm charts** to the chart repository tagged with the release version (e.g., `v1.2.3`) and `latest`.
    > `latest` is configurable 
  - **[Release TypeScript SDK](workflows/release-typescript-sdk.yaml):**  
    Updates and **publishes the TypeScript SDK** to its registry:
    - **Version:** Defined in [package.json](../sdk/typescript/package.json).

### Releasing Options: 
1. **Automatic:** Push a Git tag in the format `vX.Y.Z` (e.g., `v1.2.3`), and the workflows handle the release, marking it as the latest.
2. **Manual:** Trigger the [release workflow](https://github.com/LF-Decentralized-Trust-labs/paladin/actions/workflows/release.yaml) via the GitHub Actions interface, specifying the version and selecting the "latest" option if needed.


## Manual Actions 🛠️
Workflows can also be triggered manually when needed. Available options include:

- **[Release Orchestrator](workflows/release.yaml)**
- **[Release Docker Images](workflows/release-images.yaml)**
- **[Release Helm Chart](workflows/release-charts.yaml)**
- **[Release TypeScript SDK](workflows/release-typescript-sdk.yaml)**
- **[Build Helm Chart](workflows/build-chart.yaml)**
- **[Build Docker Images](workflows/build-image.yaml)**  
- **[Cross-Platform Docker Image Build](workflows/cross-build-images.yaml)**  
 