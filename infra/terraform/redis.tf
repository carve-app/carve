resource "aws_elasticache_subnet_group" "main" {
  name       = "${local.name_prefix}-redis"
  subnet_ids = aws_subnet.public[*].id
}

# AUTH token for Redis. URL-safe (special = false) so it embeds in the
# rediss:// connection string without escaping. AUTH requires transit
# encryption, so the two are enabled together below.
resource "random_password" "redis_auth" {
  length  = 48
  special = false
}

# A replication group (not aws_elasticache_cluster) is required for in-transit
# encryption + AUTH. Single node keeps cost flat versus the old cluster.
resource "aws_elasticache_replication_group" "main" {
  replication_group_id = "${local.name_prefix}-redis"
  description          = "${local.name_prefix} redis"
  engine               = "redis"
  engine_version       = "7.1"
  node_type            = var.redis_node_type
  num_cache_clusters   = 1
  parameter_group_name = "default.redis7"
  port                 = 6379

  subnet_group_name  = aws_elasticache_subnet_group.main.name
  security_group_ids = [aws_security_group.redis.id]

  transit_encryption_enabled = true
  at_rest_encryption_enabled = true
  auth_token                 = random_password.redis_auth.result

  snapshot_retention_limit = 1
}

locals {
  # rediss:// (TLS) with the AUTH token embedded.
  redis_url = "rediss://:${random_password.redis_auth.result}@${aws_elasticache_replication_group.main.primary_endpoint_address}:6379"
}
