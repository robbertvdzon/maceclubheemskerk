# syntax=docker/dockerfile:1
FROM golang:1.27.1-alpine AS toolchain
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

FROM toolchain AS build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM scratch AS runtime
LABEL org.opencontainers.image.source="https://github.com/robbertvdzon/maceclubheemskerk"
COPY --from=toolchain /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/server /server
USER 10001:0
EXPOSE 8080
ENTRYPOINT ["/server"]
