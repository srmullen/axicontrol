---
name: axicontrol-architecture-spec-01-device-access
description: How the k8s-deployed service gets physical access to the USB-attached AxiDraw plotter
lane: backlog
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
parent: axicontrol-architecture-spec
---

## Question

How does a k8s-deployed axicontrol service get physical access to the USB-attached AxiDraw plotter? Cover: whether pods need node affinity/pinning to the machine with the USB device, what k8s primitive exposes the device (hostPath, device plugin, privileged container, DaemonSet), and how this interacts with running the axidraw CLI as a subprocess.
