resource "aws_ecs_cluster" "main" {
  name = "${local.name_prefix}-cluster"

  setting {
    name  = "containerInsights"
    value = "disabled" # paid feature; flip to enabled past 10k MAU
  }
}

resource "aws_ecs_cluster_capacity_providers" "main" {
  cluster_name = aws_ecs_cluster.main.name

  capacity_providers = ["FARGATE", "FARGATE_SPOT"]

  default_capacity_provider_strategy {
    capacity_provider = "FARGATE_SPOT"
    weight            = 4
  }
  default_capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
    base              = 1
  }
}

# ── CloudWatch log groups ─────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "service" {
  for_each          = toset(local.ecr_repos)
  name              = "/ecs/${local.name_prefix}/${each.value}"
  retention_in_days = 14
}

# ── Service discovery for api -> nlp (private internal DNS) ──────────────────

resource "aws_service_discovery_private_dns_namespace" "internal" {
  name = "${var.project}.internal"
  vpc  = aws_vpc.main.id
}

resource "aws_service_discovery_service" "nlp" {
  name = "nlp"

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.internal.id
    dns_records {
      ttl  = 10
      type = "A"
    }
    routing_policy = "MULTIVALUE"
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}

# ── Task definitions ──────────────────────────────────────────────────────────

locals {
  ssm_secret = { for name in [
    "DATABASE_URL", "REDIS_URL", "JWT_SECRET", "NLP_INTERNAL_SECRET",
    "FORVO_API_KEY", "DEEPL_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY",
    "GOOGLE_AI_API_KEY", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET",
    "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY",
    "SENTRY_DSN", "POSTHOG_API_KEY", "SMTP_PASSWORD",
    ] : name => "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${local.ssm_prefix}/${name}"
  }
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${local.name_prefix}-api"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.ecs_task_cpu
  memory                   = var.ecs_task_memory
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.task_api.arn

  container_definitions = jsonencode([{
    name      = "api"
    image     = "${aws_ecr_repository.service["api"].repository_url}:${var.api_image_tag}"
    essential = true

    portMappings = [{ containerPort = 8080, protocol = "tcp" }]

    environment = [
      { name = "PORT", value = "8080" },
      { name = "NLP_SERVICE_URL", value = "http://nlp.${aws_service_discovery_private_dns_namespace.internal.name}:8001" },
      { name = "MEDIA_SERVICE_URL", value = "https://${local.media_domain}" },
      { name = "APP_BASE_URL", value = local.web_origin },
      { name = "ALLOWED_ORIGINS", value = "${local.web_origin},https://www.${var.domain}" },
      { name = "SMTP_HOST", value = "email-smtp.${var.aws_region}.amazonaws.com" },
      { name = "SMTP_PORT", value = "587" },
      { name = "SMTP_FROM", value = "noreply@${var.domain}" },
    ]

    secrets = [
      { name = "DATABASE_URL", valueFrom = local.ssm_secret["DATABASE_URL"] },
      { name = "REDIS_URL", valueFrom = local.ssm_secret["REDIS_URL"] },
      { name = "JWT_SECRET", valueFrom = local.ssm_secret["JWT_SECRET"] },
      { name = "NLP_INTERNAL_SECRET", valueFrom = local.ssm_secret["NLP_INTERNAL_SECRET"] },
      { name = "STRIPE_SECRET_KEY", valueFrom = local.ssm_secret["STRIPE_SECRET_KEY"] },
      { name = "STRIPE_WEBHOOK_SECRET", valueFrom = local.ssm_secret["STRIPE_WEBHOOK_SECRET"] },
      { name = "SENTRY_DSN", valueFrom = local.ssm_secret["SENTRY_DSN"] },
      { name = "DEEPL_API_KEY", valueFrom = local.ssm_secret["DEEPL_API_KEY"] },
      { name = "OPENAI_API_KEY", valueFrom = local.ssm_secret["OPENAI_API_KEY"] },
      { name = "ANTHROPIC_API_KEY", valueFrom = local.ssm_secret["ANTHROPIC_API_KEY"] },
      { name = "GOOGLE_AI_API_KEY", valueFrom = local.ssm_secret["GOOGLE_AI_API_KEY"] },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.service["api"].name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "api"
      }
    }
  }])
}

resource "aws_ecs_task_definition" "nlp" {
  family                   = "${local.name_prefix}-nlp"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 512
  memory                   = 1024
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.task_nlp.arn

  container_definitions = jsonencode([{
    name      = "nlp"
    image     = "${aws_ecr_repository.service["nlp"].repository_url}:${var.nlp_image_tag}"
    essential = true

    portMappings = [{ containerPort = 8001, protocol = "tcp" }]

    environment = [
      { name = "PORT", value = "8001" },
      { name = "DICT_DB_PATH", value = "/app/data/dictionary.db" },
    ]

    secrets = [
      { name = "NLP_INTERNAL_SECRET", valueFrom = local.ssm_secret["NLP_INTERNAL_SECRET"] },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.service["nlp"].name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "nlp"
      }
    }
  }])
}

resource "aws_ecs_task_definition" "media" {
  family                   = "${local.name_prefix}-media"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.ecs_task_cpu
  memory                   = var.ecs_task_memory
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.task_media.arn

  container_definitions = jsonencode([{
    name      = "media"
    image     = "${aws_ecr_repository.service["media"].repository_url}:${var.media_image_tag}"
    essential = true

    portMappings = [{ containerPort = 8002, protocol = "tcp" }]

    environment = [
      { name = "PORT", value = "8002" },
      { name = "STORAGE_BACKEND", value = "r2" },
      { name = "R2_ACCOUNT_ID", value = var.cloudflare_account_id },
      { name = "R2_BUCKET", value = cloudflare_r2_bucket.media.name },
      { name = "R2_PUBLIC_BASE", value = "https://${local.media_domain}" },
      { name = "ALLOWED_ORIGINS", value = "${local.web_origin},https://www.${var.domain}" },
    ]

    secrets = [
      { name = "R2_ACCESS_KEY_ID", valueFrom = local.ssm_secret["R2_ACCESS_KEY_ID"] },
      { name = "R2_SECRET_ACCESS_KEY", valueFrom = local.ssm_secret["R2_SECRET_ACCESS_KEY"] },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.service["media"].name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "media"
      }
    }
  }])
}

# ── Services ──────────────────────────────────────────────────────────────────

resource "aws_ecs_service" "api" {
  name            = "api"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.ecs_desired_count
  launch_type     = null # use cluster default capacity provider strategy

  capacity_provider_strategy {
    capacity_provider = "FARGATE_SPOT"
    weight            = 4
  }
  capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
    base              = 1
  }

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  depends_on = [aws_lb_listener_rule.api]
}

resource "aws_ecs_service" "nlp" {
  name            = "nlp"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.nlp.arn
  desired_count   = var.ecs_desired_count

  capacity_provider_strategy {
    capacity_provider = "FARGATE_SPOT"
    weight            = 1
  }

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = true
  }

  service_registries {
    registry_arn = aws_service_discovery_service.nlp.arn
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }
}

resource "aws_ecs_service" "media" {
  name            = "media"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.media.arn
  desired_count   = var.ecs_desired_count

  capacity_provider_strategy {
    capacity_provider = "FARGATE_SPOT"
    weight            = 4
  }
  capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
    base              = 1
  }

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.media.arn
    container_name   = "media"
    container_port   = 8002
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  depends_on = [aws_lb_listener_rule.media]
}
