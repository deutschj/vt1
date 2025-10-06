# syntax=docker/dockerfile:1.6
ARG TARGETOS
ARG TARGETARCH

FROM --platform=$BUILDPLATFORM golang:1.22-bookworm AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
# If your main is in ./, keep "."; otherwise set correct path
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/server .

FROM --platform=$TARGETPLATFORM gcr.io/distroless/static-debian12
COPY --from=builder --chown=65532:65532 --chmod=0755 /out/server /server
WORKDIR /
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/server"]

