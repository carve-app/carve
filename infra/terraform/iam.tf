# ── ECS execution role (pulls images, reads secrets, writes logs) ────────────

data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "ecs_execution" {
  name               = "${local.name_prefix}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy_attachment" "ecs_execution_managed" {
  role       = aws_iam_role.ecs_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# Allow execution role to read our SSM secrets so the `secrets` block in task
# defs can inject them as env vars.
data "aws_iam_policy_document" "ecs_execution_ssm" {
  statement {
    actions = [
      "ssm:GetParameters",
      "ssm:GetParameter",
      "kms:Decrypt",
    ]
    resources = [
      "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter/${var.project}/${var.env}/*",
      "arn:aws:kms:${var.aws_region}:${data.aws_caller_identity.current.account_id}:key/*",
    ]
  }
}

resource "aws_iam_role_policy" "ecs_execution_ssm" {
  role   = aws_iam_role.ecs_execution.id
  policy = data.aws_iam_policy_document.ecs_execution_ssm.json
}

# ── ECS task roles (application IAM — what the running container can do) ─────
# Per-service so we can scope narrowly.

resource "aws_iam_role" "task_api" {
  name               = "${local.name_prefix}-task-api"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role" "task_nlp" {
  name               = "${local.name_prefix}-task-nlp"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role" "task_media" {
  name               = "${local.name_prefix}-task-media"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

# API can send email via SES.
data "aws_iam_policy_document" "task_api_ses" {
  statement {
    actions = [
      "ses:SendEmail",
      "ses:SendRawEmail",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "task_api_ses" {
  role   = aws_iam_role.task_api.id
  policy = data.aws_iam_policy_document.task_api_ses.json
}

# Media doesn't talk to AWS storage (R2 is via S3 API w/ access keys), but it
# may write CloudWatch logs via the SDK. No extra perms needed beyond defaults.
