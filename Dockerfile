FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/proxy ./cmd/proxy

FROM alpine:3.19

RUN addgroup -S ainrve && adduser -S -G ainrve ainrve

WORKDIR /app

COPY --from=builder /out/proxy /usr/local/bin/proxy
COPY --chown=ainrve:ainrve ainrve.yaml.example /app/ainrve.yaml.example

EXPOSE 8080

USER ainrve

ENTRYPOINT ["/usr/local/bin/proxy"]
