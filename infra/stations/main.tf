# Registers the ground station fleet through the custom Terraform provider.
# Reused against the local API (dev_overrides) and the AWS API Gateway URL.
terraform {
  required_providers {
    groundstation = {
      source = "registry.terraform.io/mini4141/groundstation"
    }
  }
}

variable "endpoint" {
  type = string
}

variable "token" {
  type      = string
  sensitive = true
}

provider "groundstation" {
  endpoint = var.endpoint
  token    = var.token
}

locals {
  stations = {
    tokyo    = { latitude = 35.68, longitude = 139.69, altitude_m = 40, band = "S", rate = 120 }
    svalbard = { latitude = 78.23, longitude = 15.39, altitude_m = 450, band = "X", rate = 90 }
    santiago = { latitude = -33.45, longitude = -70.67, altitude_m = 520, band = "UHF", rate = 60 }
  }
}

resource "groundstation_station" "this" {
  for_each           = local.stations
  name               = each.key
  latitude           = each.value.latitude
  longitude          = each.value.longitude
  altitude_m         = each.value.altitude_m
  min_elevation_deg  = 5
  band               = each.value.band
  rate_per_min_minor = each.value.rate
}

output "station_ids" {
  value = { for k, s in groundstation_station.this : k => s.id }
}
