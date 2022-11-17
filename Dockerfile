FROM alpine:3.16

RUN addgroup -S ory -g 10014; \
    adduser -S ory -G ory -D -H -s /bin/nologin -u 10014
RUN apk --no-cache --upgrade --latest add ca-certificates

COPY . /usr/bin/hydra

# set up nsswitch.conf for Go's "netgo" implementation
# - https://github.com/golang/go/blob/go1.9.1/src/net/conf.go#L194-L275
RUN [ ! -e /etc/nsswitch.conf ] && echo 'hosts: files dns' > /etc/nsswitch.conf

# By creating the sqlite folder as the ory user, the mounted volume will be owned by ory:ory, which
# is required for read/write of SQLite.
RUN mkdir -p /var/lib/sqlite && \
    chown ory:ory /var/lib/sqlite

USER 10014

ENTRYPOINT ["hydra"]
CMD ["serve", "all"]
