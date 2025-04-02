# XPOS is a free and open-source tool that allows local servers to be accessible on the public internet. It supports TCP protocols like HTTP, SSH, and more.

## Installation

```shell
curl -fsSL https://xpos-it.com/install.sh | sudo bash
```

## Authentication
Obtain an authentication token from xpos-it.com/auth and run:

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

Debugging HTTP Tunnels
Enable debug mode:

```shell
xpos http 3000 --debug
```
