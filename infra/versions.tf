terraform {
  required_version = ">= 1.6"

  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "~> 2.58"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "scaleway" {
  region = var.region
  zone   = var.zone

  # Credentials come from the standard SCW_* environment variables
  # (or ~/.scwrc); see docs/DEPLOY.md.
}

variable "region" {
  description = "Scaleway region"
  type        = string
  default     = "fr-par"
}

variable "zone" {
  description = "Scaleway zone"
  type        = string
  default     = "fr-par-1"
}

variable "project_id" {
  description = "Scaleway Project ID (defaults to the SCW_PROJECT_ID env var / default project)"
  type        = string
  default     = ""
}

variable "image_tag" {
  description = "Container image tag to deploy (git SHA)"
  type        = string
}

variable "registry_namespace_name" {
  type    = string
  default = "tenttiarkisto"
}

variable "container_name" {
  type    = string
  default = "tenttiarkisto"
}

variable "bucket_name" {
  type    = string
  default = "tenttiarkisto-exams"
}

variable "db_name" {
  type    = string
  default = "tenttiarkisto"
}

variable "db_min_cpu" {
  description = "Serverless SQL DB minimum CPU (scales to zero)"
  type        = number
  default     = 0
}

variable "db_max_cpu" {
  description = "Serverless SQL DB maximum CPU"
  type        = number
  default     = 8
}

variable "db_version" {
  description = "PostgreSQL major version"
  type        = string
  default     = "16"
}

variable "container_min_scale" {
  type    = number
  default = 0
}

variable "container_max_scale" {
  type    = number
  default = 3
}

variable "container_memory_limit_bytes" {
  description = "Must match a supported memory tier (128/256/512/1024/2048 MB)"
  type        = number
  default     = 268435456 # 256 MB
}

variable "container_cpu_limit" {
  description = "Millicores matching the memory tier (256 MB -> 140m)"
  type        = number
  default     = 140
}

variable "geo_gate_enabled" {
  description = "Block writes and auth pages for non-Finnish IP addresses"
  type        = bool
  default     = true
}

variable "geo_gate_extra_cidrs" {
  description = "Extra allowed CIDRs (e.g. office ranges), comma separated"
  type        = string
  default     = ""
}