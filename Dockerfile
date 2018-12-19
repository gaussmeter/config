FROM golang AS builder
COPY ./src/config.go ./config.go
RUN mkdir -p /go/src/github.com/docker/ && cd /go/src/github.com/docker && git clone --depth 1 https://github.com/docker/docker
#RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main . 
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -a -installsuffix cgo -o config . 
FROM scratch AS main
COPY --from=builder /go/config ./config 
COPY ./src/ ./ 
CMD ["./config"]
