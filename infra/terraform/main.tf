data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  name_prefix = "${var.project}-${var.env}"
  azs         = slice(data.aws_availability_zones.available.names, 0, 2)

  api_domain          = "${var.api_subdomain}.${var.domain}"
  media_domain        = "${var.media_subdomain}.${var.domain}"
  media_upload_domain = "${var.media_upload_subdomain}.${var.domain}"
  web_origin          = "https://${var.domain}"
}
