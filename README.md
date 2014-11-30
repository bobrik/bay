# Docker registry bay

Enables distributed image pull with torrent via transparent proxy.

**WARNING: this is super-duper experimental POC, don't run it in production yet.**

## Components

### Tracker

Tracker downloads image from the registry and creates torrent distribution.
You would probably want to run several of them for high availability.

### Proxy

Proxy sits between docker and the registry, usually on the same server
where docker is running. It acts as a transparent proxy for metadata,
but enables p2p download of image layers with the help of trackers.

## How to make it work

This was only tested with self-hosted docker registry.

Start tracker somewhere, let it be `192.168.0.3:8080` for http
and `192.168.0.3:6881` for torrent tracker:

```
go run src/github.com/bobrik/bay/tracker/tracker.go \
  -tracker 192.168.0.3:6881 -listen 192.168.0.3:8080 -root /tmp/bay-tracker
```

Start proxy on `127.0.0.1` for registry `docker.example.com`:

```
go run src/github.com/bobrik/bay/proxy/proxy.go \
  -tracker http://192.168.0.3:8080/ -registry http://docker.example.com/ \
  -listen 192.168.0.3:80 -root /tmp/bay-proxy
```

Point `docker.example.com` to `127.0.0.1` (bay proxy), you can do that
in `/etc/hosts`:

```
127.0.0.1 docker.example.com
```

The next `docker pull` will use tracker and proxy to download layers.
