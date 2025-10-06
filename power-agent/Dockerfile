FROM golang:1.24 AS build
WORKDIR /src
COPY /src .
ENV CGO_ENABLED=0 GOOS=linux GOARCH=arm64
RUN go build -o /out/power-agent .

FROM debian:bookworm-slim
COPY --from=build /out/power-agent /power-agent
USER 65532:65532
ENTRYPOINT ["/power-agent"]
