ui = true

listener "tcp" {
  address       = "0.0.0.0:8200"
  tls_cert_file = "/vault/tls/tls.crt"
  tls_key_file  = "/vault/tls/tls.key"
  tls_min_version = "tls13"
}

storage "file" {
  path = "/vault/data"
}

api_addr = "https://127.0.0.1:8200"

# Telemetry
telemetry {
  prometheus_retention_time = "30s"
  disable_hostname = true
}

# Disable mlock for Docker (not supported without IPC_LOCK)
disable_mlock = true
