---
title: udp overlay listener
state: horizon
created: 2026-07-24
tags: [feature]
milestone: v0.2.x
---

A packet-conn (UDP) variant of the SDK's overlay `Listen`/`Dial`. Today's primitives are stream-shaped (`net.Listener`/`net.Conn`, so http/tcp only) and reject `udp`. A separate design — a packet-conn surface over the overlay.
