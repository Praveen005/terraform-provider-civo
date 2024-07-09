variable "credential_file" {}

terraform {
  required_providers {
    civo = {
      source = "civo/civo"
    }
  }
}

provider "civo" {
  credential_file = var.credential_file
  region = "LON1"
}

resource "civo_instance" "web" {
  # ...
}