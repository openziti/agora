# this builds docker.io/openziti/agora
FROM registry.access.redhat.com/ubi10/ubi-minimal

ARG ARTIFACTS_DIR=./dist
# e.g. linux
ARG TARGETOS
# e.g. arm64
ARG TARGETARCH

### Required OpenShift Labels
LABEL name="openziti/agora" \
      maintainer="support@netfoundry.io" \
      vendor="NetFoundry" \
      summary="Run the Agora overlay network" \
      description="Native zero-trust overlay network for agent-to-agent communication" \
      org.opencontainers.image.description="Native zero-trust overlay network for agent-to-agent communication" \
      org.opencontainers.image.source="https://github.com/openziti/agora"

### add licenses to this directory
RUN mkdir -p -m0755 /licenses
COPY ./LICENSE /licenses/apache.txt

RUN mkdir -p /usr/local/bin
COPY --chmod=0755 ${ARTIFACTS_DIR}/${TARGETARCH}/${TARGETOS}/agora \
      /usr/local/bin/

USER 65534:65534
ENTRYPOINT [ "agora" ]
