output "api_url" {
  value = aws_apigatewayv2_api.api.api_endpoint
}

output "console_url" {
  value = "https://${aws_cloudfront_distribution.web.domain_name}"
}

output "cloudfront_id" {
  value = aws_cloudfront_distribution.web.id
}

output "user_pool_id" {
  value = aws_cognito_user_pool.operators.id
}

output "user_pool_client_id" {
  value = aws_cognito_user_pool_client.console.id
}

output "cognito_domain" {
  value = "${aws_cognito_user_pool_domain.hosted.domain}.auth.${var.region}.amazoncognito.com"
}

output "iot_endpoint" {
  value = data.aws_iot_endpoint.data.endpoint_address
}

output "mission_bucket" {
  value = aws_s3_bucket.mission.bucket
}

output "web_bucket" {
  value = aws_s3_bucket.web.bucket
}

output "queue_url" {
  value = aws_sqs_queue.events.url
}

output "schedule_group" {
  value = aws_scheduler_schedule_group.passes.name
}

output "live_table" {
  value = aws_dynamodb_table.live.name
}

output "db_cluster" {
  value = aws_rds_cluster.db.cluster_identifier
}

output "ecr_repos" {
  value = { for k, r in aws_ecr_repository.svc : k => r.repository_url }
}

output "lambda_names" {
  value = { for k, f in aws_lambda_function.fn : k => f.function_name }
}

output "iot_certificate_ids" {
  value = { for k, c in aws_iot_certificate.station : k => c.id }
}

output "station_certs" {
  sensitive = true
  value     = { for k, c in aws_iot_certificate.station : k => { cert_pem = c.certificate_pem, private_key = c.private_key } }
}
