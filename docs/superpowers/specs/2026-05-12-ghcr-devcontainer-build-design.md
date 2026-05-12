# GHCR Devcontainer Build Handoff Design

## Goal

Move devcontainer image construction out of local developer machines and into GitHub Actions. Local commands should pull the GitHub-built image from GHCR and start the privileged development container from that image.

## Decisions

- Publish devcontainer images to GHCR.
- Use branch-based image tags.
- Convert branch names into Docker-compatible tag slugs, for example `feat/bad` becomes `feat-bad`.
- Local pull failures should fail the command immediately.
- Do not fall back to local image builds.

## Architecture

Add a GitHub Actions workflow that builds `.devcontainer/Dockerfile` and pushes the image to GHCR.

The default image reference is:

```text
ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>
```

The workflow derives `<owner>/<repo>` from the GitHub repository context and derives `<branch-slug>` from the branch ref. The local `Makefile` derives the same branch slug from the current git branch unless the caller overrides `DEV_IMAGE`.

## Local command behavior

`make docker` becomes a pull-only target. It should:

1. require either Docker or Podman,
2. compute the default GHCR image reference,
3. run `docker pull` or `podman pull`,
4. fail with a clear message if the image cannot be pulled.

`make exec` should keep the current container startup behavior, but its missing-image path should call the pull-only `make docker` target rather than building locally.

## Data flow

```text
GitHub branch push or manual workflow
  -> GitHub Actions builds .devcontainer/Dockerfile
  -> GHCR receives ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>
  -> local make exec computes the same branch slug
  -> local docker/podman pull
  -> privileged devcontainer starts with the mounted workspace
```

## Error handling

If the local pull fails, the command should explain that the GHCR image is unavailable and that the user should wait for or run the GitHub workflow for the current branch. It should not run `docker build` or `podman build` as a fallback.

## Documentation updates

Update repository documentation that describes the devcontainer workflow. The docs should explain that the image is built by GitHub Actions, published to GHCR with branch tags, and pulled locally with `make docker` or implicitly by `make exec`.

## Validation

Local validation should cover Makefile syntax and the generated image reference. Full end-to-end validation of pushing to GHCR requires GitHub Actions execution in the remote repository.

Application build targets such as `make backend`, `make frontend`, `make wrapper`, and `make all` are out of scope.
