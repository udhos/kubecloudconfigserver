#
# STEP 1 build executable binary
#

FROM golang:1.21.3-alpine3.18 as builder
RUN apk update
RUN apk add git
#RUN apk add --no-cache git
#RUN apk add git

RUN adduser -D -g '' user
COPY ./cmd/ /tmp/kubecloudconfigserver/cmd
COPY go.* /tmp/kubecloudconfigserver/
RUN find /tmp/kubecloudconfigserver
WORKDIR /tmp/kubecloudconfigserver
RUN go mod tidy
RUN go env -w CGO_ENABLED=0
RUN go build -o /bin/kubeconfigserver ./cmd/kubeconfigserver

#
# STEP 2 build a small image from scratch
#
# start from scratch
# FROM scratch
# COPY --from=builder /etc/passwd /etc/passwd
# COPY --from=builder /bin/sqspipe /bin/sqspipe
# USER user
# ENTRYPOINT ["/bin/sqsuser"]

#
# STEP 2 build a small image from alpine
#
FROM alpine:3.18.4
COPY --from=builder /bin/kubeconfigserver /bin/kubeconfigserver
RUN apk update
RUN apk add curl
RUN apk upgrade libssl3 libcrypto3 busybox busybox-binsh ssl_client libcurl nghttp2-libs
RUN adduser -D -g '' user
USER user
ENTRYPOINT ["/bin/kubeconfigserver"]
