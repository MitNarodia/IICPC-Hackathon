resource "aws_s3_bucket" "raw_uploads" {
  bucket = "${var.name_prefix}-raw-uploads"

  tags = {
    project = "iicpc-track1"
  }
}

resource "aws_s3_bucket_public_access_block" "raw_uploads" {
  bucket                  = aws_s3_bucket.raw_uploads.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "raw_uploads" {
  bucket = aws_s3_bucket.raw_uploads.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "raw_uploads" {
  bucket = aws_s3_bucket.raw_uploads.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

output "raw_uploads_bucket" {
  value = aws_s3_bucket.raw_uploads.bucket
}
