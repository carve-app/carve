# Cloudflare Phase 1: Pages project/domains, API DNS, and R2 buckets.

resource "cloudflare_pages_project" "web" {
  account_id        = var.cloudflare_account_id
  name              = var.pages_project_name
  production_branch = var.github_branch

  build_config {
    build_command   = "npm ci && npm run build"
    destination_dir = "build"
    root_dir        = "apps/web"
  }
}

resource "cloudflare_pages_domain" "apex" {
  account_id   = var.cloudflare_account_id
  project_name = cloudflare_pages_project.web.name
  domain       = var.domain
}

resource "cloudflare_pages_domain" "www" {
  account_id   = var.cloudflare_account_id
  project_name = cloudflare_pages_project.web.name
  domain       = "www.${var.domain}"
}

resource "cloudflare_record" "web_apex" {
  zone_id = var.cloudflare_zone_id
  name    = var.domain
  type    = "CNAME"
  content = cloudflare_pages_project.web.subdomain
  ttl     = 1
  proxied = true
}

resource "cloudflare_record" "web_www" {
  zone_id = var.cloudflare_zone_id
  name    = "www"
  type    = "CNAME"
  content = cloudflare_pages_project.web.subdomain
  ttl     = 1
  proxied = true
}

resource "cloudflare_record" "api" {
  zone_id = var.cloudflare_zone_id
  name    = var.api_subdomain
  type    = "A"
  content = aws_eip.app.public_ip
  ttl     = 1
  proxied = true
}

resource "cloudflare_record" "media_upload" {
  zone_id = var.cloudflare_zone_id
  name    = var.media_upload_subdomain
  type    = "A"
  content = aws_eip.app.public_ip
  ttl     = 1
  proxied = true
}

resource "cloudflare_r2_bucket" "media" {
  account_id = var.cloudflare_account_id
  name       = var.r2_media_bucket_name
  location   = var.r2_location
}

resource "cloudflare_r2_bucket" "backups" {
  account_id = var.cloudflare_account_id
  name       = var.r2_backup_bucket_name
  location   = var.r2_location
}
