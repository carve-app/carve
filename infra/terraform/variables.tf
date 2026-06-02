variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "env" {
  type    = string
  default = "prod"
}

variable "project" {
  type    = string
  default = "carve"
}

variable "domain" {
  description = "Apex domain managed in Cloudflare."
  type        = string
  default     = "carve.app"
}

variable "api_subdomain" {
  type    = string
  default = "api"
}

variable "media_subdomain" {
  type    = string
  default = "media"
}

variable "cloudflare_account_id" {
  description = "Cloudflare account ID (find under any zone > Overview)."
  type        = string
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID for the apex domain."
  type        = string
}

variable "github_owner" {
  type    = string
  default = "carve-app"
}

variable "github_repo" {
  type    = string
  default = "carve"
}

# ── Compute sizing (solo/demo defaults; bump for higher tiers) ────────────────

variable "ecs_task_cpu" {
  description = "Fargate CPU units per task (256 = 0.25 vCPU)."
  type        = number
  default     = 256
}

variable "ecs_task_memory" {
  description = "Fargate memory MB per task."
  type        = number
  default     = 512
}

variable "ecs_desired_count" {
  type    = number
  default = 1
}

variable "rds_instance_class" {
  type    = string
  default = "db.t4g.micro"
}

variable "rds_allocated_storage_gb" {
  type    = number
  default = 20
}

variable "redis_node_type" {
  type    = string
  default = "cache.t4g.micro"
}

# ── Image tags (deploy CI overrides per push) ─────────────────────────────────

variable "api_image_tag" {
  type    = string
  default = "latest"
}

variable "nlp_image_tag" {
  type    = string
  default = "latest"
}

variable "media_image_tag" {
  type    = string
  default = "latest"
}
