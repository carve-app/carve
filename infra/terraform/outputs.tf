output "ecr_repository_urls" {
  value = { for k, r in aws_ecr_repository.service : k => r.repository_url }
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.main.name
}

output "ecs_service_names" {
  value = {
    api   = aws_ecs_service.api.name
    nlp   = aws_ecs_service.nlp.name
    media = aws_ecs_service.media.name
  }
}

output "alb_dns_name" {
  value = aws_lb.main.dns_name
}

output "api_url" {
  value = "https://${local.api_domain}"
}

output "media_url" {
  value = "https://${local.media_domain}"
}

output "rds_endpoint" {
  value     = aws_db_instance.postgres.endpoint
  sensitive = true
}

output "redis_endpoint" {
  value = aws_elasticache_cluster.main.cache_nodes[0].address
}

output "r2_bucket_name" {
  value = cloudflare_r2_bucket.media.name
}

output "github_deploy_role_arn" {
  description = "Add this as the AWS_DEPLOY_ROLE_ARN secret in GitHub repo settings."
  value       = aws_iam_role.github_deploy.arn
}

output "subnet_ids" {
  description = "Used by CI to run migration tasks."
  value       = aws_subnet.public[*].id
}

output "ecs_tasks_security_group_id" {
  description = "Used by CI to run migration tasks."
  value       = aws_security_group.ecs_tasks.id
}

output "smtp_endpoint" {
  value = "email-smtp.${var.aws_region}.amazonaws.com:587"
}
