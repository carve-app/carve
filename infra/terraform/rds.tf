resource "random_password" "db" {
  length  = 32
  special = false
}

resource "aws_db_subnet_group" "main" {
  name       = "${local.name_prefix}-db"
  subnet_ids = aws_subnet.public[*].id
}

resource "aws_db_instance" "postgres" {
  identifier        = "${local.name_prefix}-pg"
  engine            = "postgres"
  engine_version    = "16.4"
  instance_class    = var.rds_instance_class
  allocated_storage = var.rds_allocated_storage_gb
  storage_type      = "gp3"
  storage_encrypted = true

  db_name  = "carve"
  username = "carve"
  password = random_password.db.result

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false

  backup_retention_period = 7
  backup_window           = "03:00-04:00"
  maintenance_window      = "Mon:04:00-Mon:05:00"
  skip_final_snapshot     = var.env != "prod"
  deletion_protection     = var.env == "prod"

  performance_insights_enabled = false
  apply_immediately            = false
}

# Connection string for services. Stored in SSM (see secrets.tf) so task
# definitions can inject it via the `secrets` block.
locals {
  database_url = "postgres://carve:${random_password.db.result}@${aws_db_instance.postgres.endpoint}/carve?sslmode=require"
}
