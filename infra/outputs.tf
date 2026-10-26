output "container_endpoint" {
  description = "Public HTTPS endpoint of the app"
  value       = try(scaleway_container.main.public_endpoint, "")
}

output "container_id" {
  value = scaleway_container.main.id
}

output "database_endpoint" {
  description = "Serverless SQL endpoint (postgres://...)"
  value       = scaleway_sdb_sql_database.main.endpoint
  sensitive   = true
}

output "database_id" {
  value = scaleway_sdb_sql_database.main.id
}

output "bucket_name" {
  value = scaleway_object_bucket.exams.name
}

output "registry_endpoint" {
  value = scaleway_registry_namespace.main.endpoint
}