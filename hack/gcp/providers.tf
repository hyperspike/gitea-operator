terraform {
	required_providers {
		google = {
			source = "hashicorp/google"
			version = ">= 5.36.0"
		}
	}
}

provider "google" {
	region      = "us-east5"
	zone        = "us-east5-a"
	credentials = "gcp-creds.json"
}
