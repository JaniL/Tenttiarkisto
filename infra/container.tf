# Container Registry for the application image.
resource "scaleway_registry_namespace" "main" {
  name        = var.registry_namespace_name
  description = "Images for the Tenttiarkisto application"
  is_public   = false
}

# Serverless Containers namespace + the application container.
resource "scaleway_container_namespace" "main" {
  name = var.container_name
}

resource "scaleway_container" "main" {
  namespace_id = scaleway_container_namespace.main.id
  name         = var.container_name
  description  = "HYY Tenttiarkisto (Go rewrite)"
  port         = 8080

  # image built & pushed with `make image && make push`
  image = "${scaleway_registry_namespace.main.endpoint}/tenttiarkisto:${var.image_tag}"

  min_scale              = var.container_min_scale
  max_scale              = var.container_max_scale
  memory_limit_bytes     = var.container_memory_limit_bytes
  cpu_limit              = var.container_cpu_limit
  timeout                = 300
  protocol               = "http1"
  https_connections_only = true

  liveness_probe {
    http {
      path = "/healthz"
    }
    failure_threshold = 5
    interval          = "30s"
    timeout           = "10s"
  }

  environment_variables = {
    PORT                        = "8080"
    S3_ENDPOINT                 = "https://s3.${var.region}.scw.cloud"
    S3_REGION                   = var.region
    S3_BUCKET                   = scaleway_object_bucket.exams.name
    GEO_GATE_ENABLED            = tostring(var.geo_gate_enabled)
    TRUSTED_FORWARDED_IP_HEADER = "X-Forwarded-For"
    GEO_GATE_EXTRA_CIDRS        = var.geo_gate_extra_cidrs
    SESSION_COOKIE_SECURE       = "true"
  }

  # NOTE: secret env vars are stored encrypted by Scaleway but DO end up in
  # the Terraform state file (state is local; keep it private).
  secret_environment_variables = {
    SESSION_SECRET = random_password.session_secret.result
    DATABASE_URL   = local.database_url
    S3_ACCESS_KEY  = scaleway_iam_api_key.s3.access_key
    S3_SECRET_KEY  = scaleway_iam_api_key.s3.secret_key
  }

  depends_on = [
    scaleway_sdb_sql_database.main,
    scaleway_iam_policy.s3_access,
  ]
}

resource "random_password" "session_secret" {
  length  = 48
  special = false
}