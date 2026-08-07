axicontrol
==========

Axicontrol is a web interface to control an AxiDraw penplotter. It will use the [axidraw cli](https://axidraw.com/doc/cli_api/#introduction)

## Container Registry

Docker images are published to the GitHub Container Registry: https://github.com/srmullen/finplan/pkgs/container/axicontrol

```
docker pull ghcr.io/srmullen/axicontrol:latest
```

The image is private; log in first with a token that has `read:packages`:

```
docker login ghcr.io -u <github-username>
```


## Requirements

- Use the axidraw cli
- Print, Pause, and Resume plots and plot layers.
- View the file that will be plotted.
- Manage the plot configuration
- Notifications when a plot completes
- Testing the axidraw.
- Easy to upload files to the server
- The application will run in a k8s cluster

Essentially all of the cli should be exposed for use somehow. 

## Open questions

- What features can be built on top of it for improved day to day operations of the axidraw?
