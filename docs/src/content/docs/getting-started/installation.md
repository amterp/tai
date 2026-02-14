---
title: Installation
description: How to install the tai CLI tool
---

## Prerequisites

Tai is written in Go. You need **Go 1.24+** installed on your machine.

If you don't have Go installed, follow the [official Go installation guide](https://go.dev/doc/install).

## Install tai

```bash
go install github.com/amterp/tai/cmd/tai@latest
```

This places the `tai` binary in your `$GOPATH/bin` directory (typically `~/go/bin`). Make sure this directory is on your `PATH`.

## Verify

```bash
tai --help
```

You should see tai's help output listing available commands. The current release is **v0.1.0**.

## Next steps

Head to the [Quick Start](/tai/getting-started/quick-start/) to set up your first config repo and install a profile.
