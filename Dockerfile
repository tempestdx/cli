FROM alpine:3.21.1

USER nobody

COPY tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
