# syntax=docker/dockerfile:1.7
FROM golang:1.23 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /server .
RUN chmod 755 /server

FROM gcr.io/distroless/static:nonroot
COPY --from=build /server /server
USER nonroot:nonroot
ENTRYPOINT ["/server"]
