# Minimal 2-AZ VPC. For solo/demo we put Fargate tasks in public subnets with
# assigned public IPs to skip the $32/mo NAT gateway. Inbound is locked down to
# the ALB security group; egress is open. Add private subnets + NAT when you
# stop wanting Fargate tasks to have public IPs (anything with PII compliance).

resource "aws_vpc" "main" {
  cidr_block           = "10.20.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
  tags                 = { Name = "${local.name_prefix}-vpc" }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "${local.name_prefix}-igw" }
}

resource "aws_subnet" "public" {
  count                   = length(local.azs)
  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(aws_vpc.main.cidr_block, 8, count.index)
  availability_zone       = local.azs[count.index]
  map_public_ip_on_launch = true
  tags                    = { Name = "${local.name_prefix}-public-${local.azs[count.index]}" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }
  tags = { Name = "${local.name_prefix}-public-rt" }
}

resource "aws_route_table_association" "public" {
  count          = length(aws_subnet.public)
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# ── Security groups ───────────────────────────────────────────────────────────

resource "aws_security_group" "alb" {
  name        = "${local.name_prefix}-alb"
  description = "ALB ingress from Cloudflare"
  vpc_id      = aws_vpc.main.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# Cloudflare edge IPs (https://www.cloudflare.com/ips-v4/). Refreshed via the
# `data` source below. Restricting ingress to Cloudflare blocks direct origin
# scrapes that bypass WAF/DDoS.
data "http" "cloudflare_ips_v4" {
  url = "https://www.cloudflare.com/ips-v4"
}

locals {
  cloudflare_ipv4 = compact(split("\n", data.http.cloudflare_ips_v4.response_body))
}

resource "aws_security_group_rule" "alb_https_from_cloudflare" {
  type              = "ingress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  cidr_blocks       = local.cloudflare_ipv4
  security_group_id = aws_security_group.alb.id
  description       = "HTTPS from Cloudflare edge"
}

resource "aws_security_group_rule" "alb_http_redirect" {
  type              = "ingress"
  from_port         = 80
  to_port           = 80
  protocol          = "tcp"
  cidr_blocks       = local.cloudflare_ipv4
  security_group_id = aws_security_group.alb.id
  description       = "HTTP from Cloudflare (redirects to 443)"
}

resource "aws_security_group" "ecs_tasks" {
  name        = "${local.name_prefix}-ecs"
  description = "Fargate tasks: ingress from ALB"
  vpc_id      = aws_vpc.main.id

  # ALB only fronts api (8080) and media (8002); nlp is internal-only. Listing
  # the exact ports (instead of 0-65535) limits a compromised task's reach.
  ingress {
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
    description     = "ALB to api"
  }
  ingress {
    from_port       = 8002
    to_port         = 8002
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
    description     = "ALB to media"
  }

  # api -> nlp over the internal service-discovery DNS (port 8001 only).
  ingress {
    from_port   = 8001
    to_port     = 8001
    protocol    = "tcp"
    self        = true
    description = "api to nlp"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "rds" {
  name   = "${local.name_prefix}-rds"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
    description     = "Postgres from ECS"
  }
}

resource "aws_security_group" "redis" {
  name   = "${local.name_prefix}-redis"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
    description     = "Redis from ECS"
  }
}

