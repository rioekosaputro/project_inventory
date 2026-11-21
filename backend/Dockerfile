# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /out/api /app/api
# Template PDF & migrasi dipakai saat runtime (ditambahkan di task berikutnya).
COPY templates /app/templates
COPY migrations /app/migrations

USER app
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/api"]
