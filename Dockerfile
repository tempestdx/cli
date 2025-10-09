FROM alpine:3.22.2

USER nobody

COPY tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
