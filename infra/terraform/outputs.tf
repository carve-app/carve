output "phase1_host" {
  description = "Set GitHub secret PHASE1_HOST to this IP, or use api_url once DNS has propagated."
  value       = aws_eip.app.public_ip
}

output "phase1_user" {
  description = "Set GitHub secret PHASE1_USER to this value."
  value       = var.vm_user
}

output "phase1_app_dir" {
  description = "Set GitHub secret PHASE1_APP_DIR to this value if not using the default."
  value       = var.phase1_app_dir
}

output "api_url" {
  value = "https://${local.api_domain}"
}

output "web_url" {
  value = local.web_origin
}

output "media_upload_url" {
  value = "https://${local.media_upload_domain}"
}

output "media_public_url" {
  description = "Attach this hostname to the media R2 bucket as a custom domain, then use it as R2_PUBLIC_BASE."
  value       = "https://${local.media_domain}"
}

output "database_url" {
  description = "Set DATABASE_URL in deploy/phase1/.env to this value."
  value       = local.database_url
  sensitive   = true
}

output "rds_endpoint" {
  value     = aws_db_instance.postgres.endpoint
  sensitive = true
}

output "r2_media_bucket_name" {
  value = cloudflare_r2_bucket.media.name
}

output "r2_backup_bucket_name" {
  value = cloudflare_r2_bucket.backups.name
}

output "cloudflare_pages_project_name" {
  value = cloudflare_pages_project.web.name
}

output "github_terraform_role_arn" {
  description = "Set GitHub secret AWS_TF_ROLE_ARN to this after the first local apply."
  value       = aws_iam_role.github_terraform.arn
}
