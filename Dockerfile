ARG GOLANG_IMAGE=golang:1.19.3-alpine3.16

ARG BUILD_IMAGE=alpine:3.16

FROM $GOLANG_IMAGE as builder

COPY . /dist/core

RUN apk add \
	git \
	make && \
	cd /dist/core && \
	go version && \
	make release_linux && \
	make import_linux && \
	make ffmigrate_linux

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
