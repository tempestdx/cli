FROM alpine:3.23.2

USER nobody

COPY tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
