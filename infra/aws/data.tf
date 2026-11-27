# ---- Aurora PostgreSQL Serverless v2 (scale to zero) ----
resource "aws_db_subnet_group" "db" {
  name       = "${local.name}-aurora"
  subnet_ids = aws_subnet.private[*].id
}

resource "aws_rds_cluster" "db" {
  cluster_identifier      = "${local.name}-aurora"
  engine                  = "aurora-postgresql"
  engine_version          = "17.6"
  database_name           = "gs"
  master_username         = "gsadmin"
  master_password         = random_password.db.result
  db_subnet_group_name    = aws_db_subnet_group.db.name
  vpc_security_group_ids  = [aws_security_group.db.id]
  storage_encrypted       = true
  skip_final_snapshot     = true
  deletion_protection     = false
  backup_retention_period = 1
  apply_immediately       = true

  serverlessv2_scaling_configuration {
    min_capacity             = 0
    max_capacity             = 1
    seconds_until_auto_pause = 300
  }
}

resource "aws_rds_cluster_instance" "db" {
  identifier         = "${local.name}-aurora-1"
  cluster_identifier = aws_rds_cluster.db.id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.db.engine
  engine_version     = aws_rds_cluster.db.engine_version
}

# ---- DynamoDB: latest telemetry frame + heartbeat per station ----
resource "aws_dynamodb_table" "live" {
  name         = "${local.name}-station-live"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "station"
  attribute {
    name = "station"
    type = "S"
  }
}

# ---- S3: mission data and the console ----
resource "aws_s3_bucket" "mission" {
  bucket        = "${local.name}-mission-${local.suffix}"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "mission" {
  bucket                  = aws_s3_bucket.mission.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket" "web" {
  bucket        = "${local.name}-console-${local.suffix}"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "web" {
  bucket                  = aws_s3_bucket.web.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ---- SQS event bus (IoT rule -> SQS -> ingest Lambda) ----
resource "aws_sqs_queue" "dlq" {
  name                      = "${local.name}-events-dlq"
  message_retention_seconds = 86400
}

resource "aws_sqs_queue" "events" {
  name                       = "${local.name}-events"
  visibility_timeout_seconds = 180
  message_retention_seconds  = 86400
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq.arn
    maxReceiveCount     = 5
  })
}
