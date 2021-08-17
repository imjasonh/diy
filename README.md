# DIY: Declarative Image YAML

WIP: describe image contents/metadata in a YAML file, and make it so.

Inspired by Bazel's rules_docker, but doesn't require Bazel.

Also inspired by all the Dockerfiles out there that are only `FROM`, `COPY`, `ADD`, etc., and never `RUN` anything, but doesn't require `docker` and a container runtime.
