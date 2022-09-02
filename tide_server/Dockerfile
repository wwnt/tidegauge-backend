ARG GOVERSION=latest
FROM golang:$GOVERSION AS builder

WORKDIR /go/src
COPY . .

#ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0
RUN cd tide_server && rm -f tide_server && go build

FROM alpine

VOLUME ["/var/tide_server"]
WORKDIR /var/tide_server

COPY --from=builder /go/src/tide_server/tide_server /bin/tide_server

CMD ["/bin/tide_server"]
