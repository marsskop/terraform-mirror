# terraform-mirror

A static webserver on GO that implements [Provider Network Mirror Protocol](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol) and has endpoints and logic to upload/update and delete providers.

## 🔥 Key Features
- Provider Network Mirror as a static website on **/providers** endpoint
- upload/update providers via POST on **/providers/{hostname}/{namespace}/{type}/upload/** endpoint, e.g.
```
curl -X POST -F file=@mirror/registry.terraform.io/hashicorp/external/terraform-provider-external_2.2.2_linux_amd64.zip https://{hostname}:{port}/providers/registry.terraform.io/hashicorp/external/upload/
```
- delete providers via DELETE on **/providers/{hostname}/{namespace}/{type}/{version}/{arch}** endpoint, e.g.
```
curl -X DELETE https://${hostname}:${port}/providers/registry.terraform.io/hashicorp/external/2.2.2/linux_amd64
```

## 📁 How to Host the Mirror
###  🐳 Run in Docker
- HTTPS server: run Docker image in **production** (*HTTPS with valid certificates conforms to Provider Network Mirror Protocol*)
```bash
docker run -d -v $(pwd)/providers:/tmp/providers -v $(pwd)/certs:/tmp/certs -p 8080:8080 marskop/terraform-mirror /usr/local/bin/terraform-mirror --dir=/tmp/providers --production --cert=/tmp/certs/fullchain.pem --key=/tmp/certs/privkey.pem
```
- HTTP server: run Docker image **locally** with debug enabled for testing purposes; HTTP server (*HTTP does not conform to Provider Network Mirror Protocol*)
```bash
docker run -d -v $(pwd)/providers:/tmp/providers -p 8080:8080 marskop/terraform-mirror /usr/local/bin/terraform-mirror --dir=/tmp/providers --debug
```
### ⚡ Run from Source
> 🔔 Make sure that you have [downloaded](https://go.dev/dl/) and installed **Go**. Version 1.18 or higher is required.
```bash
git clone https://github.com/marsskop/terraform-mirror.git
cd terraform-mirror
go run main.go
```
### ⚙️ CLI Arguments
```
  -dir string
        Directory to store providers in (default "providers")
  -port int
        Server port (default 8080)
  -cert string
        Path to cert file for TLS (default "cert.pem")
  -key string
        Path to key file for TLS (default "key.pem")
  -production bool
        Production mode which enables TLS and uses certificates (default false)
  -debug bool
        Debug mode (default false)
```

## 📂 How to Use the Mirror
- create ~/.terraformrc file with configuration
```
provider_installation {
  network_mirror {
    url = "${hostname}:${port}/providers/"
    include = ["registry.terraform.io/*/*"]
  }
  direct {
    exclude = ["registry.terraform.io/*/*"]
  }
}
```

## 🔧  How to Prepare and Upload Providers
*examples shown for [rancher/rke provider](https://registry.terraform.io/providers/rancher/rke/latest)*
1. For any terraform provider (from terraform.registry.io).
    - create versions.tf with required providers, e.g.
    ```
    terraform {
    required_providers {
        rke = {
        source = "rancher/rke"
        version = "1.3.0"
        }
    }
    required_version = ">= 0.13"
    }
    ```
    - make sure you have access to registry.terraform.io and run
    ```bash
    terraform providers mirror -platform=linux_amd64 mirror
    ```
    - upload provider from the mirror directory
    ```bash
    curl -X POST -F file=@mirror/registry.terraform.io/rancher/rke/terraform-provider-rke_1.3.0_linux_amd64.zip https://${hostname}:${port}/providers/registry.terraform.io/rancher/rke/upload/
    ```
2. For some providers (from their GitHub releases page).
    - download provider release from GitHub, e.g. for [terraform-provider-rke](https://github.com/rancher/terraform-provider-rke)
    ```bash
    curl -LJO https://github.com/rancher/terraform-provider-rke/releases/download/v1.4.0/terraform-provider-rke_1.4.0_linux_amd64.zip
    ```
    - upload provider
    ```bash
    curl -X POST -F file=@terraform-provider-rke_1.4.0_linux_amd64.zip https://${hostname}:${port}/providers/registry.terraform.io/rancher/rke/upload/
    ```

## 💥 But Why?
Suppose you need:
- a static mirror inside a network that does **not** have access to registry.terraform.io
- some way to store and distribute your custom provider

To work with providers uploaded to Terraform's official registry one needs Provider Network Mirror Protocol (as you do not have GPG signing keys for providers that are, well, not yours). Add an endpoint for uploading and deleting and you have a small mirror-registry in your hands.

Repos of interest:
- [Citizen](https://github.com/outsideris/citizen)
- [Terralist](https://github.com/valentindeaconu/terralist)
- [terraform-registry](https://github.com/nrkno/terraform-registry)

## ⚠️  License
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](https://www.tldrlegal.com/license/mit-license)
