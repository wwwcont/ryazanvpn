# syntax=docker/dockerfile:1.7

FROM golang:1.24.1-alpine AS builder
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/service ./${SERVICE}

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /out/service /app/service
EXPOSE 8080
ENTRYPOINT ["/app/service"]
