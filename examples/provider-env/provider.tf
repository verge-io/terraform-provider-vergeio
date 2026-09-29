terraform {
  required_providers {
    vergeio = {
      source = "verge-io/vergeio"
    }
  }
}

# Every argument is optional. Omitted values are read from the environment:
# VERGEOS_HOST
# VERGEOS_API_KEY, or VERGEOS_USERNAME and VERGEOS_PASSWORD
# VERGEOS_INSECURE, or VERGEOS_VERIFY_SSL=false
# VERGEOS_TIMEOUT (seconds)
# A value set in this block wins over the environment variable.
provider "vergeio" {}
