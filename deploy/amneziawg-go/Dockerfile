FROM alpine:3.20

RUN apk add --no-cache wireguard-tools iproute2 iptables bash dumb-init ca-certificates kmod
COPY deploy/amneziawg-go/entrypoint.sh /usr/local/bin/amnezia-entrypoint.sh
COPY deploy/amneziawg-go/awg-wrapper.sh /usr/local/bin/awg
RUN chmod +x /usr/local/bin/amnezia-entrypoint.sh /usr/local/bin/awg

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/amnezia-entrypoint.sh"]
