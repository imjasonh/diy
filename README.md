# DIY: Declarative Image YAML

Describe image contents and metadata in a YAML file, and make it so.

**This is an experimental work in progress. Don't use it for anything serious. If you want something like this, let me know how you would use it.**

Inspired by Bazel's rules_docker, but doesn't require Bazel.

If you have a Dockerfile that only contains `FROM`, `COPY`, `ADD`, etc., and never `RUN`, then you might be able to use this.

This tool doesn't require a container runtime like `docker build` does, which may make it safer/better to use in a containerized CI environment.

It doesn't currently enforce reproducibililty, but it might help with it, and could be extended to reject non-reproducible inputs in the future.

## `DIY_REPO` env var

You must set the `DIY_REPO` env var to the image repository you want to push to.
This will be combined with the image name in the config to produce the desired image location.

With `DIY_REPO=gcr.io/foo`, the config below will push the image to `gcr.io/foo/test/image`.

## Example

See [`config.yaml`](./config.yaml) for full usage.

```yaml
name: test/image

base: gcr.io/distroless/static:nonroot

layers:
- archive: https://partner-images.canonical.com/oci/impish/20210817/ubuntu-impish-oci-amd64-root.tar.gz
  sha256: 7dec15764407aeb0cebd5840798c15651bc13a7ded07c70fe2699051311baa50

- files:
  - name: hello
    contents: hello
  - name: world
    contents: world
```

```console
$ go run ./ build
```

TODO:
- fetch and install .debs
- enforce better security/reproducibility
- build OCI images by default
- cache remote archives for speed
- support building manifest lists / OCI indexes, possibly with templating
- kontain.me
