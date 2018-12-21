FROM gaussmeter/builder AS builder
COPY ./src/config.go ./config.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build -a -installsuffix cgo -o config .
FROM gaussmeter/nothing AS main
COPY --from=builder /go/config ./config
COPY ./src/ ./
CMD ["./config"]
