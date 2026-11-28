# Each station is an IoT thing with its own X.509 certificate and a policy limiting it to its own topics.
data "aws_iot_endpoint" "data" {
  endpoint_type = "iot:Data-ATS"
}

resource "aws_iot_thing" "station" {
  for_each = toset(var.stations)
  name     = each.key
  attributes = {
    project = "ground-station-cloud"
  }
}

resource "aws_iot_certificate" "station" {
  for_each = toset(var.stations)
  active   = true
}

resource "aws_iot_policy" "station" {
  for_each = toset(var.stations)
  name     = "${local.name}-station-${each.key}"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = "iot:Connect", Resource = "arn:aws:iot:${var.region}:${local.account_id}:client/agent-${each.key}" },
      {
        Effect   = "Allow"
        Action   = ["iot:Publish", "iot:RetainPublish"] # RetainPublish: retained status heartbeat and last-will
        Resource = [for k in ["telemetry", "status", "events"] : "arn:aws:iot:${var.region}:${local.account_id}:topic/gs/${each.key}/${k}"]
      },
      { Effect = "Allow", Action = "iot:Subscribe", Resource = "arn:aws:iot:${var.region}:${local.account_id}:topicfilter/gs/${each.key}/cmd" },
      { Effect = "Allow", Action = "iot:Receive", Resource = "arn:aws:iot:${var.region}:${local.account_id}:topic/gs/${each.key}/cmd" },
    ]
  })
}

resource "aws_iot_policy_attachment" "station" {
  for_each = toset(var.stations)
  policy   = aws_iot_policy.station[each.key].name
  target   = aws_iot_certificate.station[each.key].arn
}

resource "aws_iot_thing_principal_attachment" "station" {
  for_each  = toset(var.stations)
  thing     = aws_iot_thing.station[each.key].name
  principal = aws_iot_certificate.station[each.key].arn
}

# topic rule: edge messages -> SQS event bus (adds the message kind taken from the topic)
resource "aws_iam_role" "iot_rule" {
  name = "${local.name}-iot-rule"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "iot.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy" "iot_rule" {
  role = aws_iam_role.iot_rule.id
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = "sqs:SendMessage", Resource = aws_sqs_queue.events.arn }]
  })
}

resource "aws_iot_topic_rule" "ingest" {
  name        = "gsc_ingest"
  enabled     = true
  sql         = "SELECT *, topic(3) AS kind FROM 'gs/+/+' WHERE topic(3) <> 'cmd'"
  sql_version = "2016-03-23"
  sqs {
    queue_url  = aws_sqs_queue.events.url
    role_arn   = aws_iam_role.iot_rule.arn
    use_base64 = false
  }
}
