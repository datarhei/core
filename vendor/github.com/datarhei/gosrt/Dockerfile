ARG BUILD_IMAGE=golang:1.23-alpine

FROM $BUILD_IMAGE as builder

COPY . /build

RUN cd /build/contrib/client && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -a -o client .
RUN cd /build/contrib/server && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -a -o server .

FROM scratch

COPY --from=builder /build/contrib/client/client /bin/srt-client
COPY --from=builder /build/contrib/server/server /bin/srt-server

WORKDIR /srt
