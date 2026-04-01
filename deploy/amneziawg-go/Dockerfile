FROM golang:1.24-alpine AS amneziawg-go-builder

RUN apk add --no-cache make git build-base linux-headers
WORKDIR /src
RUN git clone --depth=1 https://github.com/amnezia-vpn/amneziawg-go.git .
RUN make

FROM alpine:3.20 AS amneziawg-tools-builder

RUN apk add --no-cache make git build-base bash linux-headers
WORKDIR /src
RUN git clone --depth=1 https://github.com/amnezia-vpn/amneziawg-tools.git .
WORKDIR /src/src
RUN make
RUN make install DESTDIR=/out WITH_WGQUICK=yes

FROM alpine:3.20

RUN apk add --no-cache \
    bash \
    dumb-init \
    iproute2 \
    iptables \
    ca-certificates \
    libstdc++

COPY --from=amneziawg-go-builder /src/amneziawg-go /usr/local/bin/amneziawg-go
COPY --from=amneziawg-tools-builder /out/usr/bin/awg /usr/local/bin/awg
COPY --from=amneziawg-tools-builder /out/usr/bin/awg-quick /usr/local/bin/awg-quick

COPY deploy/amneziawg-go/entrypoint.sh /usr/local/bin/amnezia-entrypoint.sh
RUN chmod +x \
    /usr/local/bin/amneziawg-go \
    /usr/local/bin/awg \
    /usr/local/bin/awg-quick \
    /usr/local/bin/amnezia-entrypoint.sh

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/amnezia-entrypoint.sh"]