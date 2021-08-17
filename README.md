# DIY: Declarative Image YAML

WIP: describe image contents/metadata in a YAML file, and make it so.

Inspired by Bazel's rules_docker, but doesn't require Bazel.

Also inspired by all the Dockerfiles out there that are only `FROM`, `COPY`, `ADD`, etc., and never `RUN` anything, but doesn't require `docker` and a container runtime.

## Example

config.yaml:

```yaml
base: gcr.io/distroless/static:nonroot

annotations:
  hey: this
  is: cool

layers:
- files:
  - name: hello
    contents: hello
  - name: world
    contents: world

- files:
  - name: seeya
    contents: fella
  - name: goodbye
    contents: later
```

```console
$ go run ./ -f config.yaml -t <my-image>
```

This will push an image as described in the YAML:

- based on gcr.io/distroless/static:nonroot
- containing the specified annotations
- containing two layers each with two files, as described
