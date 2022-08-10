#
# STEP 1 build executable binary
#

FROM golang:1.19-alpine3.16 as builder
RUN apk update
RUN apk add git
#RUN apk add --no-cache git
#RUN apk add git

RUN adduser -D -g '' user
COPY ./cmd/ /tmp/kubecloudconfigserver/cmd
COPY ./env/ /tmp/kubecloudconfigserver/env
COPY ./refresh/ /tmp/kubecloudconfigserver/refresh
COPY go.* /tmp/kubecloudconfigserver/
RUN find /tmp/kubecloudconfigserver
WORKDIR /tmp/kubecloudconfigserver
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /bin/kubeconfigserver ./cmd/kubeconfigserver

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
FROM alpine:3.16
COPY --from=builder /bin/kubeconfigserver /bin/kubeconfigserver
RUN apk add curl
RUN adduser -D -g '' user
USER user
ENTRYPOINT ["/bin/kubeconfigserver"]
