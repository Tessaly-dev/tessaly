# SPDX-License-Identifier: Apache-2.0
# The tess image. GoReleaser builds it from the release binaries, so it holds
# no build tools. See docs/wiki/Docker.md.
#
# The image has git, gh and ssh, but no agent CLI. Build your own image FROM
# this one and install the agents you use.
FROM debian:trixie-slim@sha256:8aa5a7bf5a1955bddbe7d74fcdd0b91ab30e74362a8b73af672b0481b35b44a0

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates git gh openssh-client \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd --create-home --uid 1000 --shell /bin/bash tess \
	&& install -d -m 0700 -o tess -g tess /data

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/tess /usr/bin/tess

USER tess
ENV TESSALY_HOME=/data
VOLUME /data
EXPOSE 7777

# Inside the container tess listens on every interface, so Docker can publish
# the port. Publish it on the host's 127.0.0.1 only: -p 127.0.0.1:7777:7777.
ENTRYPOINT ["/usr/bin/tess"]
CMD ["serve", "--listen", "0.0.0.0", "--port", "7777"]
