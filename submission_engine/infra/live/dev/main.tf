terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

module "network" {
  source      = "../../modules/network"
  name_prefix = var.name_prefix
  cidr_block  = var.cidr_block
}

module "object_storage" {
  source      = "../../modules/object-storage"
  name_prefix = var.name_prefix
}

output "raw_uploads_bucket" {
  value = module.object_storage.raw_uploads_bucket
}
