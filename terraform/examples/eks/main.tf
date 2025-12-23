# Example EKS workspace used by the orchestration flow; replace with real resources.
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
  description = "VPC ID for the cluster"
}

variable "subnet_ids" {
  type        = list(string)
  description = "Subnet IDs for the cluster"
}

output "cluster_name" {
  value       = "example-eks-cluster"
  description = "Dummy cluster name (replace with real resource output)"
}

