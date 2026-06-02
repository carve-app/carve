terraform {
  required_version = ">= 1.6"

  backend "s3" {
    # Backend config provided at `terraform init -backend-config=backend.hcl`.
    # See infra/terraform/bootstrap/ for the bucket + lock table.
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.40"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    http = {
      source  = "hashicorp/http"
      version = "~> 3.4"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project   = "carve"
      ManagedBy = "terraform"
      Env       = var.env
    }
  }
}

provider "cloudflare" {
  # Reads CLOUDFLARE_API_TOKEN from env. Token needs:
  #   Zone:DNS:Edit on the carve.app zone
  #   Account:Workers R2 Storage:Edit
  #   Account:Cloudflare Pages:Edit
}
