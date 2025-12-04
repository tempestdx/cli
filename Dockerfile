FROM alpine:3.23.0

USER nobody

COPY tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
