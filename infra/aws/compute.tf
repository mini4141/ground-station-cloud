# ---- ECR (images are pushed by scripts/deploy.sh before the Lambdas are created) ----
resource "aws_ecr_repository" "svc" {
  for_each             = local.services
  name                 = "${local.name}-${each.key}"
  force_delete         = true
  image_tag_mutability = "MUTABLE"
}

# ---- log groups with 1-day retention, destroyed with the stack ----
resource "aws_cloudwatch_log_group" "lambda" {
  for_each          = local.services
  name              = "/aws/lambda/${local.name}-${each.key}"
  retention_in_days = 1
}

resource "aws_cloudwatch_log_group" "domain_events" {
  name              = "/aws/events/${local.name}-domain-events"
  retention_in_days = 1
}

# ---- IAM ----
data "aws_iam_policy_document" "lambda_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  for_each           = local.services
  name               = "${local.name}-${each.key}-lambda"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

resource "aws_iam_role_policy_attachment" "vpc" {
  for_each   = local.services
  role       = aws_iam_role.lambda[each.key].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

resource "aws_iam_role_policy_attachment" "xray" {
  for_each   = local.services
  role       = aws_iam_role.lambda[each.key].name
  policy_arn = "arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess"
}

locals {
  scheduler_arn  = "arn:aws:lambda:${var.region}:${local.account_id}:function:${local.name}-scheduler" # static: a reference to the function would be a cycle
  iot_cmd_topics = "arn:aws:iot:${var.region}:${local.account_id}:topic/gs/*/cmd"
  live_rw        = ["dynamodb:GetItem", "dynamodb:UpdateItem", "dynamodb:PutItem", "dynamodb:DescribeTable"]
}

resource "aws_iam_role_policy" "api" {
  role = aws_iam_role.lambda["api"].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = local.live_rw, Resource = aws_dynamodb_table.live.arn },
      { Effect = "Allow", Action = "s3:PutObject", Resource = "${aws_s3_bucket.mission.arn}/*" },
      { Effect = "Allow", Action = "iot:Publish", Resource = local.iot_cmd_topics },
      { Effect = "Allow", Action = "events:PutEvents", Resource = "arn:aws:events:${var.region}:${local.account_id}:event-bus/default" },
      { Effect = "Allow", Action = ["scheduler:CreateSchedule"], Resource = "arn:aws:scheduler:${var.region}:${local.account_id}:schedule/${aws_scheduler_schedule_group.passes.name}/*" },
      { Effect = "Allow", Action = "iam:PassRole", Resource = aws_iam_role.scheduler.arn },
    ]
  })
}

resource "aws_iam_role_policy" "scheduler_fn" {
  role = aws_iam_role.lambda["scheduler"].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = "iot:Publish", Resource = local.iot_cmd_topics },
      { Effect = "Allow", Action = "events:PutEvents", Resource = "arn:aws:events:${var.region}:${local.account_id}:event-bus/default" },
    ]
  })
}

resource "aws_iam_role_policy" "ingest" {
  role = aws_iam_role.lambda["ingest"].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = local.live_rw, Resource = aws_dynamodb_table.live.arn },
      { Effect = "Allow", Action = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"], Resource = aws_sqs_queue.events.arn },
    ]
  })
}

# ---- Lambda functions (container images; the ADOT collector extension is baked into the image) ----
locals {
  database_url = "postgres://${aws_rds_cluster.db.master_username}:${random_password.db.result}@${aws_rds_cluster.db.endpoint}:5432/${aws_rds_cluster.db.database_name}?sslmode=require"
  common_env = {
    DATABASE_URL                        = local.database_url
    OTEL_EXPORTER_OTLP_ENDPOINT         = "http://localhost:4318"
    OPENTELEMETRY_COLLECTOR_CONFIG_FILE = "/opt/collector-config/config.yaml"
    LIVE_TABLE                          = aws_dynamodb_table.live.name
    EVENTS                              = "eventbridge"
    UPLOAD_SECRET                       = random_password.upload.result
    PLAN_LEAD_MIN                       = tostring(var.plan_lead_min)
    PUBLISHER                           = "iot"
    IOT_ENDPOINT                        = data.aws_iot_endpoint.data.endpoint_address
    API_PUBLIC_URL                      = aws_apigatewayv2_api.api.api_endpoint
  }
  fn_env = {
    api = {
      MISSION_BUCKET       = aws_s3_bucket.mission.bucket
      TRUST_GATEWAY_JWT    = "true"
      SCHEDULER_LAMBDA_ARN = local.scheduler_arn
      SCHEDULER_ROLE_ARN   = aws_iam_role.scheduler.arn
      SCHEDULE_GROUP       = aws_scheduler_schedule_group.passes.name
    }
    ingest    = { RUN_MIGRATIONS = "false" }
    scheduler = { RUN_MIGRATIONS = "false" }
  }
}

resource "aws_lambda_function" "fn" {
  for_each      = local.services
  function_name = "${local.name}-${each.key}"
  role          = aws_iam_role.lambda[each.key].arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.svc[each.key].repository_url}:${var.image_tag}"
  memory_size   = 512
  timeout       = each.key == "api" ? 60 : 30

  vpc_config {
    subnet_ids         = [aws_subnet.private[0].id]
    security_group_ids = [aws_security_group.lambda.id]
  }
  tracing_config {
    mode = "Active"
  }
  environment {
    variables = each.key == "scheduler" || each.key == "api" ? merge(local.common_env, local.fn_env[each.key]) : merge({
      DATABASE_URL                        = local.database_url
      LIVE_TABLE                          = aws_dynamodb_table.live.name
      OTEL_EXPORTER_OTLP_ENDPOINT         = "http://localhost:4318"
      OPENTELEMETRY_COLLECTOR_CONFIG_FILE = "/opt/collector-config/config.yaml"
    }, local.fn_env[each.key])
  }
  depends_on = [
    aws_cloudwatch_log_group.lambda,
    aws_iam_role_policy_attachment.vpc,
    aws_iam_role_policy.api,
    aws_iam_role_policy.ingest,
    aws_iam_role_policy.scheduler_fn,
    aws_vpc_endpoint.interface,
    aws_rds_cluster_instance.db,
  ]
}

resource "aws_lambda_event_source_mapping" "ingest" {
  event_source_arn                   = aws_sqs_queue.events.arn
  function_name                      = aws_lambda_function.fn["ingest"].arn
  batch_size                         = 10
  maximum_batching_window_in_seconds = 1
  function_response_types            = ["ReportBatchItemFailures"]
  scaling_config {
    maximum_concurrency = 2
  }
}

# ---- EventBridge: one-time pass-reminder schedules (created per booking by the API) + domain events ----
resource "aws_scheduler_schedule_group" "passes" {
  name = "${local.name}-passes"
}

resource "aws_iam_role" "scheduler" {
  name = "${local.name}-scheduler-invoke"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "scheduler.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy" "scheduler_invoke" {
  role = aws_iam_role.scheduler.id
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = "lambda:InvokeFunction", Resource = local.scheduler_arn }]
  })
}

resource "aws_cloudwatch_event_rule" "domain" {
  name = "${local.name}-domain-events"
  event_pattern = jsonencode({
    source = ["groundstation.platform"]
  })
}

data "aws_iam_policy_document" "events_logs" {
  statement {
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.domain_events.arn}:*"]
    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com", "delivery.logs.amazonaws.com"]
    }
  }
}

resource "aws_cloudwatch_log_resource_policy" "events" {
  policy_name     = "${local.name}-events-to-logs"
  policy_document = data.aws_iam_policy_document.events_logs.json
}

resource "aws_cloudwatch_event_target" "domain_logs" {
  rule = aws_cloudwatch_event_rule.domain.name
  arn  = aws_cloudwatch_log_group.domain_events.arn
}

# ---- API Gateway HTTP API: Cognito JWT authorizer, public health and agent upload routes ----
resource "aws_apigatewayv2_api" "api" {
  name          = "${local.name}-api"
  protocol_type = "HTTP"
  cors_configuration {
    allow_origins = ["https://${aws_cloudfront_distribution.web.domain_name}", "http://localhost:5173"]
    allow_methods = ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"]
    allow_headers = ["authorization", "content-type", "x-upload-token"]
    max_age       = 3600
  }
}

resource "aws_apigatewayv2_integration" "api" {
  api_id                 = aws_apigatewayv2_api.api.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.fn["api"].invoke_arn
  payload_format_version = "2.0"
  timeout_milliseconds   = 30000
}

resource "aws_apigatewayv2_authorizer" "cognito" {
  api_id           = aws_apigatewayv2_api.api.id
  name             = "cognito"
  authorizer_type  = "JWT"
  identity_sources = ["$request.header.Authorization"]
  jwt_configuration {
    audience = [aws_cognito_user_pool_client.console.id]
    issuer   = "https://cognito-idp.${var.region}.amazonaws.com/${aws_cognito_user_pool.operators.id}"
  }
}

resource "aws_apigatewayv2_route" "secured" {
  api_id             = aws_apigatewayv2_api.api.id
  route_key          = "$default"
  target             = "integrations/${aws_apigatewayv2_integration.api.id}"
  authorization_type = "JWT"
  authorizer_id      = aws_apigatewayv2_authorizer.cognito.id
}

resource "aws_apigatewayv2_route" "public" {
  for_each           = toset(["GET /healthz", "POST /bookings/{id}/upload-url"])
  api_id             = aws_apigatewayv2_api.api.id
  route_key          = each.key
  target             = "integrations/${aws_apigatewayv2_integration.api.id}"
  authorization_type = "NONE"
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.api.id
  name        = "$default"
  auto_deploy = true
}

resource "aws_lambda_permission" "apigw" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.fn["api"].function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.api.execution_arn}/*/*"
}
