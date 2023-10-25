# homelab-gitops (WIP)

This is a hacky program I use to deploy podman containers to some CoreOS VMs, it works for my use but I probably wouldn't use it for anything serious.

It clones a repo with manifests, checks that the latest commit is signed, and runs containers configured in a configuration file and manifests.

The repo it clones should look like this:

```
> tree .
.
└── gitops
    └── hostname_a
        ├── service_a
        │   ├── manifest.yml
        │   └── service_a_config_file
        ├── service_b
        │   ├── manifest.yml
        │   └── service_b_config_file
        └── config.yml
> cat gitops/hostname_a/config.yml
pre:
  script: |
    mkdir -p /var/mnt/volumes/caddy
services:
  service_a: {}
  service_b: {}
> cat gitops/hostname_a/service_a/manifest.yml
Container:
  ContainerName:
    - caddy
  Image:
    - "docker.io/library/caddy:2.5.2-alpine"
  Volume:
    - "/var/mnt/volumes/caddy:/data:z"
    - "${SERVICE_DIR}/service_a_config_file:/etc/caddy/Caddyfile:z"
  Network:
    - host
  HostName:
    - hostname_a
```

The fields in the manifest corresponds to the fields in [podman container unit files](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html#container-units-container). The way the program runs services is by:

1. Cloning the repo.
2. Reading the configuration for the current host (`./gitops/$HOSTNAME/config.yml`).
3. Read the "manifest" for each service defined in the config (`./gitops/$HOSTNAME/$SERVICE/manifest.yml`).
4. Create a hash from all the files in the "service directory".
5. Generate a podman container unit file (`$HOME/.config/containers/systemd/gitops-$SERVICE.container`) from the manifest, with a container label containing the "service hash", and the service name.
6. Check all running containers, if a container is running with a label indicating the same service, but a different hash (or no running container is found), start/restart the service with `systemctl --user restart gitops-$SERVICE.service`.
7. If a running container has a label indicating a service that isn't listed in the configuration file, stop it.

If you need to add secrets to the manifest, you can create a file `manifest.sops.yml` using [sops](https://github.com/getsops/sops), and provide a way for the server to decrypt it using a `SOPS_*` environment variable. The configuration in the encrypted file will be "merged" with the normal manifest.

The program is meant to be run in a service with a systemd timer as a non-root user.

Dependencies between services are automatically handled by systemd, if `service_a` depends on `service_b`, you can add `gitops-service_b.service` to `Unit.Requires` in the manifest of `service_a`. This will for example make sure they are started in the correct order.
