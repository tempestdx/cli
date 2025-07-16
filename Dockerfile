FROM alpine:3.22.1

USER nobody

COPY tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
