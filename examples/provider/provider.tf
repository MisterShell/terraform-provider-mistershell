terraform {
  required_providers {
    mistershell = {
      source = "mistershell/mistershell"
    }
  }
  required_version = ">= 1.5.0"
}

provider "mistershell" {
  url      = "https://mistershell.example.com" # or set MISTERSHELL_URL
  api_key  = var.mistershell_api_key           # or set MISTERSHELL_API_KEY
  insecure = true                              # for self-signed certs
}

variable "mistershell_api_key" {
  type      = string
  sensitive = true
}
