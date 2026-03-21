job "nomad-gateway" {
  node_pool   = "default"
  datacenters = ["dc1"]
  type        = "service"

  group "nomad-gateway" {
    count = 1

    network {
      port "http" {
        to = 8080
      }
    }

    service {
      name     = "nomad-gateway"
      port     = "http"
      provider = "consul"
      tags = [
        "traefik.enable=true",
        "traefik.http.routers.nomad-gateway.rule=Host(`nomad-gateway.example.com`)",
        "traefik.http.routers.nomad-gateway.entrypoints=websecure",
        "traefik.http.routers.nomad-gateway.tls=true",
      ]

      check {
        type     = "http"
        path     = "/health"
        port     = "http"
        interval = "30s"
        timeout  = "5s"

        check_restart {
          limit = 3
          grace = "30s"
        }
      }
    }

    restart {
      attempts = 3
      interval = "2m"
      delay    = "15s"
      mode     = "fail"
    }

    vault {
      cluster     = "default"
      change_mode = "noop"
    }

    task "nomad-gateway" {
      driver = "docker"

      config {
        image = "ghcr.io/lobo235/nomad-gateway:latest"
        ports = ["http"]
      }

      # Secrets pulled from Vault at secret/data/nomad-gateway
      template {
        data = <<EOF
NOMAD_TOKEN={{ with secret "secret/data/nomad-gateway" }}{{ .Data.data.nomad_token }}{{ end }}
GATEWAY_API_KEY={{ with secret "secret/data/nomad-gateway" }}{{ .Data.data.api_key }}{{ end }}
EOF
        destination = "secrets/nomad-gateway.env"
        env         = true
      }

      env {
        NOMAD_ADDR = "https://nomad.example.com"
        PORT       = "8080"
      }

      resources {
        cpu    = 500
        memory = 256
      }

      kill_timeout = "35s"
    }
  }
}
