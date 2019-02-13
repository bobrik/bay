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
Docker hub works a little bit differently than self-hosted
registry to make it work with p2p easily.

Let's run tracker on `192.168.0.1` (`host1`) and proxies on `192.168.0.{2,3,4}` (`host{2,3,4}`).

```bash
host1> docker run -d --net=host bobrik/bay-tracker \
  -listen 192.168.0.1:8888 -tracker 192.168.0.1:6881 -root /tmp
```

Now let's run local proxies on each box:

```bash
host2> docker run -d -p 127.0.0.1:80:80 bobrik/bay-proxy \
  -tracker http://192.168.0.1:8888/ -listen :80 -root /tmp

host3> docker run -d -p 127.0.0.1:80:80 bobrik/bay-proxy \
  -tracker http://192.168.0.1:8888/ -listen :80 -root /tmp

host4> docker run -d -p 127.0.0.1:80:80 bobrik/bay-proxy \
  -tracker http://192.168.0.1:8888/ -listen :80 -root /tmp
```

In `/etc/hosts` on each machine add the next record:

```
127.0.0.1 p2p-<my-registry.com>
```

where `my-registry.com` should be your usual registry.

After that on `host{2,3,4}` you can run:


```bash
docker pull p2p-<my-registry.com>/myimage
```

and it will work just like

```bash
docker pull <my-registry.com>/myimage
```

but with p2p magic and unicorns.

## Related Projects

 - https://github.com/jackpal/Taipei-Torrent
