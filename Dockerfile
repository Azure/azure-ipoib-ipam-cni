FROM golang as builder

COPY . /workspace/
WORKDIR /workspace/
RUN make build

FROM gcr.io/distroless/static-debian12
COPY --from=builder /workspace/bin/* /
ENTRYPOINT ["/install"]