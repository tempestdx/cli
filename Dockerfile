FROM alpine:3.21.2

USER nobody

COPY tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
