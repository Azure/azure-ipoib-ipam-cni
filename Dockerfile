FROM golang as builder

COPY . /workspace/
WORKDIR /workspace/
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/azure-ipoib-ipam-cni ./cmd/ && \
    CGO_ENABLED=0 GOOS=linux go build -o bin/install ./cmd/install && \
    CGO_ENABLED=0 GOOS=linux go build -o bin/webhook ./cmd/webhook

FROM gcr.io/distroless/static-debian12
COPY --from=builder /workspace/bin/* /
ENTRYPOINT ["/install"]