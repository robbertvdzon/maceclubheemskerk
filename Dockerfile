# syntax=docker/dockerfile:1
FROM golang:1.27.1-alpine AS toolchain
RUN apk add --no-cache ffmpeg
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN mkdir /runtime-data && chmod 0770 /runtime-data
COPY cmd ./cmd
COPY internal ./internal

FROM toolchain AS build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate-sqlite ./cmd/migrate-sqlite

FROM alpine:3.24 AS runtime
RUN apk add --no-cache ffmpeg ca-certificates
LABEL org.opencontainers.image.source="https://github.com/robbertvdzon/maceclubheemskerk"
COPY --from=toolchain /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/server /server
COPY --from=build /out/migrate-sqlite /migrate-sqlite
COPY --from=toolchain --chown=10001:0 /runtime-data /data
COPY --from=toolchain --chown=10001:0 /runtime-data /videos
USER 10001:0
EXPOSE 8080
ENTRYPOINT ["/server"]
