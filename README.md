# oci-registry-mirror

> Automatically sync and secure the container images your infrastructure depends on from public registries into your private registry.

OCI Registry Mirror is a CLI tool built that automates mirroring images from public registries (like Docker Hub) to private registries using `skopeo`.

Its purpose is to centralize the images you use, avoiding external rate limits and ensuring that every image is scanned before being copied.

## Configuration File (`images.yaml`)

The tool read configuration entries from a local `images.yaml` file, e.g.:

```yaml
images:
  - name: golang
    source: docker.io/library/golang
    destination: myprivatecontaineregistry.com/docker-hub/originals/golang
    tag: "1.26.5"
    ignore-severities: true # Optional: skip failure block if image has vulnerabilities
```

## Setup

To run OCI Registry Mirror, configure the following environment variables:

- Mandatory:
  - `REGISTRY_USERNAME` - Private registry username
  - `REGISTRY_PASSWORD` - Private registry password

## Usage

### Security Scan

Scan all listed images for HIGH and CRITICAL vulnerabilities using Trivy.

```bash
oci-registry-mirror scan
```

**Note:** The scanner runs through all images in the file, accumulates the results, and displays a summary at the end. If any image fails and doesn't have `ignore-severities: true`, the runtime fails.

### Run Mirror

Start copying the missing images to your private destination registry.

```bash
oci-registry-mirror mirror
```

Optional:

- `--dry-run` - Show what would be copied without executing actual image copies.

## Automation with Renovate

You can automate image updates by coupling this tool with [Renovate](https://docs.renovatebot.com). Add the following custom regex manager to your `renovate.json` so it can track and update your `images.yaml` automatically:

```json
{
  "$schema": "[https://docs.renovatebot.com/renovate-schema.json](https://docs.renovatebot.com/renovate-schema.json)",
  "customManagers": [
    {
      "customType": "regex",
      "managerFilePatterns": [
        "/^images\\.yaml$/"
      ],
      "matchStrings": [
        "- name:\\s*[^\\n]+\\n\\s*source:\\s*(?<depName>[^\\s]+)[\\s\\S]*?\\n\\s*tag:\\s*\"?(?<currentValue>[^\"\\s]+)\"?"
      ],
      "datasourceTemplate": "docker"
    }
  ]
}
```

## CI Example (GitLab Pipeline)

This pipeline setup closes the automation loop:

- **On Feature Branches / Renovate MRs:** It runs a dry-run check and executes the security scan. If it passes, the MR can be automatically merged.

- **On the Main Branch:** It runs the actual mirror process to sync the images to the private registry.

```yaml
stages:
  - test
  - deploy

variables:
  REGISTRY_USERNAME: $REGISTRY_USERNAME
  REGISTRY_PASSWORD: $REGISTRY_PASSWORD

# --- Verification for Feature Branches ---
verify-images-mr:
  stage: test
  tags:
    - docker
  image:
    name: ghcr.io/ghdrope/oci-registry-mirror:latest
    entrypoint: [""]
  script:
    - oci-registry-mirror run --dry-run
    - oci-registry-mirror scan
  rules:
    - if: $CI_COMMIT_BRANCH !=$CI_DEFAULT_BRANCH

# --- Production Pushes ---
mirror-images-main:
  stage: deploy
  tags:
    - docker
  image:
    name: ghcr.io/ghdrope/oci-registry-mirror:latest
    entrypoint: [""]
  script:
    - oci-registry-mirror run
  rules:
    - if: $CI_COMMIT_BRANCH ==$CI_DEFAULT_BRANCH
```
