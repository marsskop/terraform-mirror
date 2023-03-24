FROM golang:1.18 as builder
ARG PROJECT=tfmirror
ENV GOPATH=/tmp/go
WORKDIR ${GOPATH}/src/${PROJECT}

COPY ./go.mod .
COPY ./go.sum .
COPY ./main.go .
COPY ./vendor vendor

RUN go build -v -o /terraform-mirror .

FROM redhat/ubi8-minimal:8.7
COPY --from=builder /terraform-mirror /usr/local/bin/terraform-mirror

USER 1000