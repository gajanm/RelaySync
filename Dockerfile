FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /relaysync-api ./cmd/relaysync-api

FROM alpine:3.19
WORKDIR /app
RUN adduser -D -g '' relaysync
USER relaysync
COPY --from=builder /relaysync-api /app/relaysync-api
COPY openapi.yaml /app/openapi.yaml
EXPOSE 8080
ENTRYPOINT ["/app/relaysync-api"]
