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
  description = "R2 custom-domain hostname. The v4 Cloudflare provider cannot attach it; see the runbook."
  type        = string
  default     = "media"
}

variable "media_upload_subdomain" {
  description = "VM-routed media service hostname for uploads/health/debug."
  type        = string
  default     = "media-upload"
}

variable "cloudflare_account_id" {
  description = "Cloudflare account ID."
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

variable "github_branch" {
  type    = string
  default = "main"
}

variable "pages_project_name" {
  type    = string
  default = "carve-web"
}

variable "vpc_cidr" {
  type    = string
  default = "10.30.0.0/16"
}

variable "ssh_public_key" {
  description = "Public SSH key installed on the Phase 1 VM for deploy/admin access."
  type        = string
}

variable "ssh_allowed_cidr_blocks" {
  description = "CIDR blocks allowed to SSH to the VM. Tighten after first access."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "vm_instance_type" {
  type    = string
  default = "t3.small"
}

variable "vm_user" {
  type    = string
  default = "ubuntu"
}

variable "phase1_app_dir" {
  type    = string
  default = "/opt/carve/phase1"
}

variable "vm_root_volume_gb" {
  type    = number
  default = 40
}

variable "db_name" {
  type    = string
  default = "carve"
}

variable "db_username" {
  type    = string
  default = "carve"
}

variable "db_engine_version" {
  type    = string
  default = "16"
}

variable "db_instance_class" {
  type    = string
  default = "db.t4g.micro"
}

variable "db_allocated_storage_gb" {
  type    = number
  default = 20
}

variable "db_backup_retention_days" {
  type    = number
  default = 7
}

variable "db_deletion_protection" {
  type    = bool
  default = true
}

variable "db_apply_immediately" {
  type    = bool
  default = false
}

variable "r2_media_bucket_name" {
  type    = string
  default = "carve-prod-media"
}

variable "r2_backup_bucket_name" {
  type    = string
  default = "carve-prod-backups"
}

variable "r2_location" {
  type    = string
  default = "ENAM"
}
