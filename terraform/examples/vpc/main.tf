# Example VPC workspace used by the orchestration flow; replace with real resources.
terraform {
  required_version = ">= 1.5.0"
}

provider "aws" {
  region = var.region
}

variable "region" {
  type        = string
  description = "AWS region"
}

output "vpc_id" {
  value       = "example-vpc-id"
  description = "Dummy VPC ID (replace with real resource output)"
}

