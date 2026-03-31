FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git
WORKDIR /src
RUN git clone --depth 1 https://github.com/amnezia-vpn/amneziawg-go.git .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/amneziawg-go .

FROM alpine:3.20

RUN apk add --no-cache wireguard-tools iproute2 iptables bash dumb-init ca-certificates
COPY --from=builder /out/amneziawg-go /usr/local/bin/amneziawg-go
COPY deploy/amneziawg-go/entrypoint.sh /usr/local/bin/amnezia-entrypoint.sh
COPY deploy/amneziawg-go/awg-wrapper.sh /usr/local/bin/awg
RUN chmod +x /usr/local/bin/amnezia-entrypoint.sh /usr/local/bin/awg

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/amnezia-entrypoint.sh"]
