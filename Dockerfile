FROM golang AS builder
RUN mkdir -p /go/src/github.com/docker/ && \
    cd /go/src/github.com/docker && \
    git clone --depth 1 https://github.com/docker/docker && \
    mkdir -p /go/src/github.com/gorilla/ && \
    cd /go/src/github.com/gorilla && \
    git clone --depth 1 https://github.com/gorilla/mux && \
    mkdir -p /go/src/github.com/dgraph-io/ && \
    cd /go/src/github.com/dgraph-io && \
    git clone --depth 1 https://github.com/dgraph-io/badger && \
    mkdir -p /go/src/github.com/AndreasBriese/ && \
    cd /go/src/github.com/AndreasBriese && \
    git clone --depth 1 https://github.com/AndreasBriese/bbloom && \
    mkdir -p /go/src/github.com/dgryski/ && \
    cd /go/src/github.com/dgryski && \
    git clone --depth 1 https://github.com/dgryski/go-farm && \
    mkdir -p /go/src/github.com/dustin/ && \
    cd /go/src/github.com/dustin && \
    git clone --depth 1 https://github.com/dustin/go-humanize && \
    mkdir -p /go/src/github.com/golang/ && \
    cd /go/src/github.com/golang && \
    git clone --depth 1 https://github.com/golang/protobuf && \
    mkdir -p /go/src/github.com/pkg/ && \
    cd /go/src/github.com/pkg && \
    git clone --depth 1 https://github.com/pkg/errors && \
    mkdir -p /go/src/github.com/rs/ && \
    cd /go/src/github.com/rs && \
    git clone --depth 1 https://github.com/rs/xid && \
    mkdir -p /go/src/golang/x/ && \
    cd /go && \
    go get -v golang.org/x/net/trace && \
    go get -v golang.org/x/sys/unix
COPY ./src/config.go ./config.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build -a -installsuffix cgo -o config .
FROM balenalib/rpi-alpine AS main
COPY --from=builder /go/config ./config
COPY ./src/ ./
CMD ["./config"]
