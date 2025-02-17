FROM alpine:3.21.3

USER nobody

COPY tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
