resource "aws_vpc" "this" {
  cidr_block           = var.cidr_block
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name    = var.name_prefix
    project = "iicpc-track1"
  }
}

output "vpc_id" {
  value = aws_vpc.this.id
}
