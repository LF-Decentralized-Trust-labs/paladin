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

- **[Test Examples](workflows/test-examples.yaml):**
  - **Status:** [![Test Examples](https://github.com/LFDT-Paladin/paladin/actions/workflows/test-examples.yaml/badge.svg?branch=main)](https://github.com/LFDT-Paladin/paladin/actions/workflows/test-examples.yaml)
  - **Trigger:** Runs on pushes and pull requests to `main` that modify files under `examples/**`.
  - **Purpose:** Validates that example changes don't break existing functionality by running them against both the latest published and local versions of the SDK and contracts.
  - **Key Steps:**
    - **Backwards Compatibility:** Runs `test-examples.yaml` with `build_local_sdk` and `build_local_abi` set to `false` to ensure the examples work with the latest published versions.
    - **Forward Compatibility:** Runs `test-examples.yaml` with `build_local_sdk` and `build_local_abi` set to `true` to ensure the examples work with the current code in the PR.

- **Test TypeScript SDK:**  
  Validation of the TypeScript SDK is performed as part of the current PR workflows to ensure changes remain compatible with examples.

- **Test Solidity Changes:**  
  Solidity contract validation is included in the PR workflows to ensure changes do not break example functionality.

All checks must pass before PRs can be merged to the main branch.

## Changes Pushed to Main
Once changes are merged into the `main` branch, workflows prepare the project for production:

- **[Build (push)](workflows/on-push.yml):**  
  Similar to PR checks, this ensures the integrity of the main branch:
  - **Build and Test:** Compiles code and runs tests for all subdirectories using [build.yaml](workflows/build.yaml).

- **[Publish Docker Images](workflows/cross-build-images.yaml):**  
  Produces production-grade, cross-platform Docker images and publishes them to the container registry:
  - **Registry:** `ghcr.io/lfdt-paladin`
  - **Images:** `paladin`, `paladin-operator`
  - **Tagging:** Images are tagged with `main`.
  - **Platforms:** `linux/amd64`, `linux/arm64`

- **[Update Documentation](workflows/docs.yaml):**  
  Detects documentation updates and publishes the latest content to the documentation site.

## Release Time
Paladin follows a two-stage release process to ensure quality and stability. All releases are triggered through the **[Release Entry Point](https://github.com/LFDT-Paladin/paladin/actions/workflows/release-entry.yaml)** workflow due to the way npm trusted publishing works.

### Stage 1: Release Candidate (RC)
Release candidates are created first for testing and validation:

- **[Release Candidate Workflow](workflows/release-candidate.yaml):**  
  Triggered via the Release Entry Point with `release_type=rc`, this workflow creates pre-releases:
  - **[Release Docker Images](workflows/release-images.yaml):**  
    Builds and **publishes Docker images** tagged with the RC version (e.g., `v1.2.3-rc.1`).
    - **Registries:** 
      - `ghcr.io/lfdt-paladin`
      - `docker.io/lfdecentralizedtrust`
    - **Images:** `paladin`, `paladin-operator`
    > RC images are **never** tagged as `latest`
  - **[Release Helm Chart](workflows/release-charts.yaml):**
    Packages and **publishes Helm charts** tagged with the RC version.
    > **Includes E2E Testing:** This workflow automatically runs comprehensive end-to-end tests before publishing
  - **[Release Solidity Contracts](workflows/release-solidity-contracts.yaml):**
    Packages contract ABIs and deployment artifacts for distribution.
  - **GitHub Release:** Creates a pre-release with all artifacts
 
### Stage 2: Final Release
Once the RC has been tested and validated, the final release can be created:

- **[Release Orchestrator](workflows/release.yaml):**  
  Triggered via the Release Entry Point with `release_type=final`, this workflow coordinates the final release:
  - **[Release Docker Images](workflows/release-images.yaml):**  
    Builds and **publishes Docker images** tagged with the release version (e.g., `v1.2.3`) and `latest`.
    - **Registries:** 
      - `ghcr.io/lfdt-paladin`
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
  - **GitHub Release:** Creates a final release with all artifacts

## How to Create a Release

All releases are triggered through the unified **[Release Entry Point](https://github.com/LFDT-Paladin/paladin/actions/workflows/release-entry.yaml)** workflow.

### Quick Start: Release Process
Follow these steps to create a new Paladin release:

#### Step 1: Create a Release Candidate
1. Go to **[Actions → Release Entry Point](https://github.com/LFDT-Paladin/paladin/actions/workflows/release-entry.yaml)**
2. Click **"Run workflow"**
3. Fill in the required fields:
   - **release_type**: Select `rc`
   - **tag**: RC tag (e.g., `v1.2.3-rc.1`)
4. Click **"Run workflow"**
5. Wait for the workflow to complete and test the RC thoroughly

#### Step 2: Create the Final Release
1. Go to **[Actions → Release Entry Point](https://github.com/LFDT-Paladin/paladin/actions/workflows/release-entry.yaml)**
2. Click **"Run workflow"**
3. Fill in the required fields:
   - **release_type**: Select `final`
   - **tag**: Final release version (e.g., `v1.2.3`)
   - **rc_tag**: The RC tag you tested (e.g., `v1.2.3-rc.1`)
   - **latest**: Check if this should be marked as the latest release
4. Click **"Run workflow"**

#### SDK-Only Release (Optional)
To release only the TypeScript SDK without a full release:
1. Go to **[Actions → Release Entry Point](https://github.com/LFDT-Paladin/paladin/actions/workflows/release-entry.yaml)**
2. Click **"Run workflow"**
3. Fill in the required fields:
   - **release_type**: Select `sdk`
   - **tag**: SDK version (e.g., `v1.2.3`)
   - **sdk_dist_tags**: NPM dist-tags, semicolon-separated (e.g., `latest` or `rc;rc.1`)
   - **ref**: (Optional) Commit ref for checkout, defaults to workflow branch
4. Click **"Run workflow"**

### Example Release Flow
```
1. Create RC:     release_type=rc, tag=v1.2.3-rc.1  → Test thoroughly
2. Final Release: release_type=final, tag=v1.2.3, rc_tag=v1.2.3-rc.1
```

### Important Release Process Notes:
- **RC Required:** Final releases must be built from a tested RC tag
- **Testing:** Always test RCs thoroughly before promoting to final release
- **Version Consistency:** The RC version must match the final version (e.g., `v1.0.0-rc.1` → `v1.0.0`)
- **Format:** RC tags must follow `vX.Y.Z-rc.W` format (e.g., `v1.2.3-rc.1`)

## Manual Docker Image Release

> ⚠️ **Note:** This is **not the standard release process**.
> Use this workflow only as a **workaround** when you need to release Docker images for testing, hotfixes, or other exceptional cases.
> Official releases should always go through the [Release Candidate](#stage-1-release-candidate-rc-) and [Final Release](#stage-2-final-release-) workflows.

Paladin includes a manual workflow for pushing Docker images to both DockerHub and GHCR.

### How to Run

1. Navigate to **Actions → [Image Release](https://github.com/LFDT-Paladin/paladin/actions/workflows/release-images.yaml)**.

2. Click **"Run workflow"**.

3. Fill in the required fields:

   * **Tag:**

     * Required.
     * The version to tag the images with (e.g., `v0.1.0-hotfix.0`).
   * **Whether to also tag the images with "latest":**

     * Optional.
     * Set to `true` only if you want this build to overwrite the `latest` tag.

4. Click **Run workflow**.

### What Happens

* Builds cross-platform Docker images (`linux/amd64`, `linux/arm64`).
* Pushes them to both:

  * **DockerHub:** `docker.io/lfdecentralizedtrust/...`
  * **GHCR:** `ghcr.io/lfdt-paladin/...`
* Tags include the version you specified, and optionally `latest`.

### Example

If you run with:

* **Tag:** `v0.1.0-hotfix.0`
* **Latest:** `false`

Images will be published as:

* `docker.io/lfdecentralizedtrust/paladin:v0.1.0-hotfix.0`
* `ghcr.io/lfdt-paladin/paladin:v0.1.0-hotfix.0`

If you set **Latest = true**, the same images will also be tagged as `latest`.


## Manual Helm Chart Release

> ⚠️ **Note:** This is **not part of the standard release process**.
> Use this workflow only as a **workaround** when you need to release a Helm chart for testing, debugging, or other exceptional cases.
> Official releases should always go through the [Release Candidate](#stage-1-release-candidate-rc-) and [Final Release](#stage-2-final-release-) workflows.

Paladin includes a manual workflow for publishing Helm charts outside of the normal release cycle.

### How to Run

1. Navigate to **Actions → [Release Helm Chart](https://github.com/LFDT-Paladin/paladin/actions/workflows/release-charts.yaml)**.

2. Click **"Run workflow"**.

3. Fill in the required fields:

   * **Branch:** Choose the branch to release from (e.g., `main` or a feature branch).
   * **Whether to mark the release as latest:** Optional.

     * Leave unchecked for testing or patch releases.
     * Check only if you explicitly want this to be marked as `latest`.
   * **The docker registry to use for the images:**

     * Options: `docker.io` or `ghcr.io` (default).
   * **The images tags to patch the chart with:**

     * Example: `main`, `test-branch`, or `v0.1.0-hotfix.0`.
     * Must match an existing built/published image tag.
   * **The helm chart tag to release the chart:**

     * Example: `v0.1.0-hotfix.0`.
     * Becomes the chart’s version in the repository.

4. Click **Run workflow**.

### What Happens

* The workflow rebuilds the CRDs and Operator charts.
* Patches image references and Helm dependencies with the provided tags.
* Runs **Helm template validation** and **E2E tests**.
* Publishes the chart to the [GitHub Pages chart repository](https://LFDT-Paladin.github.io/paladin).
* Uploads deployment artifacts (`basenet.yaml`, `devnet.yaml`, `customnet.yaml`, etc.) for download.

### Example

If you run with:

* **Images tag:** `test-branch`
* **Helm chart tag:** `v0.1.0-hotfix.0`

You will publish a Helm chart version `0.11.0-fix.0` that points to images built with the tag `test-branch`.


## Manual Actions 🛠️
Workflows can also be triggered manually when needed. Available options include:

- **[Release Entry Point](workflows/release-entry.yaml)** (RC, final, and SDK releases)
- **[Test Rollout](workflows/test-rollout.yaml)** (Version rollout testing)
- **[Release Docker Images](workflows/release-images.yaml)**
- **[Release Helm Chart](workflows/release-charts.yaml)**
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
