terraform {
  required_version = ">= 1.9"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "aws" {
  region  = var.region
  profile = "ground-station-cloud-deployer"
  default_tags {
    tags = {
      Project   = "ground-station-cloud"
      Ephemeral = "true"
      ManagedBy = "terraform"
      Owner     = "alvieirvansyah"
    }
  }
}

variable "region" {
  type    = string
  default = "ap-southeast-2"
}

variable "image_tag" {
  type        = string
  default     = "latest"
  description = "Tag of the Lambda container images pushed to ECR by scripts/deploy.sh"
}

variable "stations" {
  type    = list(string)
  default = ["tokyo", "svalbard", "santiago"]
}

variable "plan_lead_min" {
  type    = number
  default = 10
}

data "aws_caller_identity" "me" {}
data "aws_availability_zones" "az" {
  state = "available"
}

resource "random_id" "suffix" {
  byte_length = 3
}

resource "random_password" "db" {
  length  = 24
  special = false
}

resource "random_password" "upload" {
  length  = 32
  special = false
}

locals {
  name       = "gsc"
  suffix     = random_id.suffix.hex
  services   = toset(["api", "ingest", "scheduler"])
  account_id = data.aws_caller_identity.me.account_id
}
