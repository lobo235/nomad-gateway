job "nomad-gateway" {
  node_pool   = "default"
  datacenters = ["dc1"]
  type        = "service"

  update {
    max_parallel     = 1
    health_check     = "checks"
    min_healthy_time = "10s"
    healthy_deadline = "5m"
    auto_revert      = true
  }

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
      change_mode = "restart"
    }

    task "nomad-gateway" {
      driver = "docker"

      config {
        image      = "gitea.example.com/example/nomad-gateway:latest"
        force_pull = true
        ports      = ["http"]
      }

      template {
        data = <<EOF
{{ with secret "kv/data/nomad/default/nomad-gateway" }}
NOMAD_TOKEN={{ .Data.data.nomad_token }}
GATEWAY_API_KEY={{ .Data.data.gateway_api_key }}
{{ end }}
EOF
        destination = "secrets/nomad-gateway.env"
        env         = true
      }

      env {
        NOMAD_ADDR = "https://nomad.example.com"
        PORT       = "8080"
      }

      template {
        data        = "{{ with nomadVar \"nomad/jobs/nomad-gateway\" }}{{ .image_digest }}{{ end }}"
        destination = "local/deploy-trigger"
        change_mode = "restart"
      }

      resources {
        cpu    = 500
        memory = 256
      }

      kill_timeout = "35s"
    }
  }
}
