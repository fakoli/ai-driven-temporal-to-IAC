# Example subnets workspace used by the orchestration flow; replace with real resources.
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

variable "vpc_id" {
  type        = string
  description = "VPC ID to attach subnets"
}

output "subnet_ids" {
  value       = ["example-subnet-a", "example-subnet-b"]
  description = "Dummy subnet IDs (replace with real resource output)"
}

