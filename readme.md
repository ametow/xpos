## XPOS is a free and open-source tool that allows local servers to be accessible on the public internet. It supports any TCP protocols like HTTP, SSH, and more.

## Installation

```shell
curl -fsSL https://xpos-io.com/install.sh | sudo bash
```

## Authentication

Request an authentication token from <https://xpos-io.com/auth> and run:

```shell
xpos auth <auth-token>
```

## Start HTTP Tunnel

Expose your local server on port 3000:

```shell
xpos http 3000
```

## Start TCP Tunnel

Expose an SSH server on port 22:

```shell
xpos tcp 22
```

## How it works

1. The agent (`xpos` CLI) dials the relay's event server and sends a `TunnelRequest` containing the protocol and auth token.
2. The relay authenticates the user, allocates listeners, and replies with a `TunnelCreated` event carrying the public address and a private callback address.
3. When a public visitor connects, the relay emits a `NewConnection` event over the control channel. The agent dials back to the relay's private address, identifies which visitor it is, and the relay bridges the two connections so bytes flow end-to-end to the local service.

## Data path

Each public visitor request travels through a chain of three TCP connections:

```
customer  <--TCP-->  relay (public)  ==bind==  relay (private)  <--TCP-->  agent (callback)  ==bind==  agent (local)  <--TCP-->  127.0.0.1:<port>
```

Sockets created per visitor:

- **Relay**: 2 accepts — one on the public listener (per-tunnel for TCP, shared `:8080` gateway for HTTP) and one on the per-tunnel private listener.
- **Agent**: 2 dials — one to the local service on `127.0.0.1:<port>` and one back to the relay's private address.

In addition, a single long-lived control connection between the agent and the relay carries all `TunnelRequest`, `TunnelCreated`, and `NewConnection` events for the session.

### If you want to contribute, please refer [to this page](/contribute.md)
