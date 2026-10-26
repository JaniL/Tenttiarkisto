# Serverless SQL Database (scales to zero; pay-per-use).
# https://www.scaleway.com/en/serverless-sql-database/

# IAM application + API key that the container uses to authenticate
# to the database (per Scaleway docs for connecting to a Serverless DB).
resource "scaleway_iam_application" "db" {
  name = "${var.container_name}-db-app"
}

resource "scaleway_iam_api_key" "db" {
  application_id = scaleway_iam_application.db.id
  description    = "Serverless SQL credentials for the ${var.container_name} container"
}

resource "scaleway_iam_policy" "db_access" {
  name           = "${var.container_name}-db-access"
  description    = "Allow the app IAM identity to use the Serverless SQL Database"
  application_id = scaleway_iam_application.db.id
  rule {
    project_ids          = [local.project_id]
    permission_set_names = ["ServerlessSQLDatabaseReadWrite"]
  }
}

resource "scaleway_sdb_sql_database" "main" {
  name    = var.db_name
  min_cpu = var.db_min_cpu
  max_cpu = var.db_max_cpu
  version = var.db_version
}

data "scaleway_account_project" "default" {
  name = "default"
}

locals {
  project_id = var.project_id != "" ? var.project_id : data.scaleway_account_project.default.id

  # endpoint is like postgres://host:port/<db>
  # credentials are <iam_application_id>:<api_key_secret_key>
  database_url = format("postgres://%s:%s@%s?sslmode=require",
    scaleway_iam_application.db.id,
    scaleway_iam_api_key.db.secret_key,
    trimprefix(scaleway_sdb_sql_database.main.endpoint, "postgres://"),
  )
}