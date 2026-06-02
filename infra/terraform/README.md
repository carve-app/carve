# Carve — AWS infrastructure (Terraform)

Sizing: **solo/demo** (~$25–40/mo cloud bill, see `docs/13-infrastructure-costs.md`). Resources scale up by tuning the variables in `variables.tf` and re-applying.

## Layout

```
bootstrap/              # one-time: TF state bucket + DynamoDB lock
versions.tf             # providers + backend declaration
variables.tf            # input variables
main.tf                 # locals + shared data sources
network.tf              # VPC, subnets, security groups
ecr.tf                  # 3 image repos
rds.tf                  # Postgres
redis.tf                # ElastiCache
iam.tf                  # ECS roles
secrets.tf              # SSM Parameter Store (placeholders for 3p keys)
alb.tf                  # ALB + ACM cert (DNS-validated via Cloudflare)
ecs.tf                  # Cluster, task defs, services, log groups
ses.tf                  # Email sending domain + SMTP user
cloudflare.tf           # R2 bucket, DNS, edge cache rules
github_oidc.tf          # GitHub Actions deploy role
outputs.tf
```

## First-time bootstrap

```sh
# 1. State bucket (local backend)
cd infra/terraform/bootstrap
terraform init
terraform apply

# Copy the backend_hcl output into ../backend.hcl

cd ..
terraform init -backend-config=backend.hcl

cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars   # set cloudflare ids

export CLOUDFLARE_API_TOKEN=...        # see docs/deploy/api-keys.md
export AWS_PROFILE=carve-admin         # any IAM principal with Admin

terraform plan
terraform apply
```

After the first apply, the **placeholder** SSM secrets need real values (see `docs/deploy/api-keys.md`):

```sh
aws ssm put-parameter --name /carve/prod/STRIPE_SECRET_KEY --value 'sk_live_...' --overwrite --type SecureString
aws ssm put-parameter --name /carve/prod/DEEPL_API_KEY     --value '...'         --overwrite --type SecureString
# etc.
```

Then trigger the CI deploy workflow to push images and roll the ECS services.

## Scaling up

| Tier | Set in `terraform.tfvars` |
|---|---|
| 1k MAU | `ecs_task_cpu = 512`, `ecs_task_memory = 1024`, `ecs_desired_count = 1`, `rds_instance_class = "db.t4g.small"` |
| 100k MAU | `ecs_desired_count = 3`, `rds_instance_class = "db.r6g.xlarge"`, set `aws_db_instance.postgres.multi_az = true` in `rds.tf`, switch ElastiCache to `aws_elasticache_replication_group` |

## Manual one-time steps (cannot be Terraformed)

1. **Cloudflare R2 custom domain** for `media.carve.app` — set in the R2 dashboard once the bucket exists (the Cloudflare TF provider doesn't yet support this resource).
2. **SES production access request** — the SES console has a "Request production access" button. Until granted, mail only goes to verified addresses.
3. **Cloudflare Pages — connect to GitHub** — one-click OAuth handshake in the Pages dashboard. The deploy workflow then pushes builds.
4. **Stripe webhook endpoint** — register `https://api.carve.app/v1/billing/webhook` in the Stripe dashboard, paste the signing secret into `STRIPE_WEBHOOK_SECRET`.
