# syntax=docker/dockerfile:1.7

FROM golang:1.24.4-alpine AS builder
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/service ./${SERVICE}

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates docker-cli
COPY --from=builder /out/service /app/service
EXPOSE 8080
ENTRYPOINT ["/app/service"]
