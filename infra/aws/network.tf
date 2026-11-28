# Private-only network: no internet gateway, no NAT. Lambdas reach AWS through VPC endpoints.
resource "aws_vpc" "main" {
  cidr_block           = "10.20.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags                 = { Name = "${local.name}-vpc" }
}

resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.20.${count.index}.0/24"
  availability_zone = data.aws_availability_zones.az.names[count.index]
  tags              = { Name = "${local.name}-private-${count.index}" }
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "${local.name}-private" }
}

resource "aws_route_table_association" "private" {
  count          = 2
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# free gateway endpoints
resource "aws_vpc_endpoint" "gateway" {
  for_each          = toset(["s3", "dynamodb"])
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${var.region}.${each.key}"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.private.id]
  tags              = { Name = "${local.name}-${each.key}" }
}

resource "aws_security_group" "lambda" {
  name        = "${local.name}-lambda"
  description = "Lambda functions"
  vpc_id      = aws_vpc.main.id
  egress {
    description = "Only VPC-internal targets exist (no NAT or IGW): Aurora, interface and gateway endpoints"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "db" {
  name        = "${local.name}-aurora"
  description = "Aurora admits the app tier only"
  vpc_id      = aws_vpc.main.id
  ingress {
    description     = "PostgreSQL from Lambda"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.lambda.id]
  }
}

resource "aws_security_group" "endpoints" {
  name        = "${local.name}-endpoints"
  description = "Interface endpoints"
  vpc_id      = aws_vpc.main.id
  ingress {
    description     = "HTTPS from Lambda"
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.lambda.id]
  }
}

# single-AZ interface endpoints (cost): SQS, IoT data plane, EventBridge Scheduler + events, X-Ray
resource "aws_vpc_endpoint" "interface" {
  for_each            = toset(["sqs", "iot.data", "scheduler", "events", "xray"])
  vpc_id              = aws_vpc.main.id
  service_name        = "com.amazonaws.${var.region}.${each.key}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [aws_subnet.private[0].id]
  security_group_ids  = [aws_security_group.endpoints.id]
  private_dns_enabled = each.key != "iot.data" # the IoT data service offers no private DNS name: Lambdas use the endpoint-specific vpce DNS name instead
  tags                = { Name = "${local.name}-${replace(each.key, ".", "-")}" }
}
