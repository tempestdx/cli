FROM alpine:3.22.0

USER nobody

COPY tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
