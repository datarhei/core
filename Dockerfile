ARG GOLANG_IMAGE=golang:1.24.3-alpine3.22@sha256:b4f875e650466fa0fe62c6fd3f02517a392123eea85f1d7e69d85f780e4db1c1
ARG BUILD_IMAGE=alpine:3.22.0@sha256:8a1f59ffb675680d47db6337b49d22281a139e9d709335b492be023728e11715

# Cross-Compilation
# https://www.docker.com/blog/faster-multi-platform-builds-dockerfile-cross-compilation-guide/
FROM --platform=$BUILDPLATFORM $GOLANG_IMAGE AS builder

ARG TARGETOS TARGETARCH TARGETVARIANT
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=$TARGETVARIANT

COPY . /dist/core

RUN apk add \
	git \
	make

RUN cd /dist/core && \
	make release && \
	make import && \
	make ffmigrate

FROM $BUILD_IMAGE

COPY --from=builder /dist/core/core /core/bin/core
COPY --from=builder /dist/core/import /core/bin/import
COPY --from=builder /dist/core/ffmigrate /core/bin/ffmigrate
COPY --from=builder /dist/core/mime.types /core/mime.types
COPY --from=builder /dist/core/run.sh /core/bin/run.sh

RUN mkdir /core/config /core/data

ENV CORE_CONFIGFILE=/core/config/config.json
ENV CORE_STORAGE_DISK_DIR=/core/data
ENV CORE_DB_DIR=/core/config

VOLUME ["/core/data", "/core/config"]
ENTRYPOINT ["/core/bin/run.sh"]
WORKDIR /core
