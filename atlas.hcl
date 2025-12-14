variable "db_url" {
  type    = string
  default = getenv("DB_URL")
}

variable "atlas_dev_url" {
  type    = string
  default = getenv("ATLAS_DEV_URL")
}

locals {
  hrm_src = [
    "file://modules/hrm/infrastructure/atlas/core_deps.sql",
    "file://modules/hrm/infrastructure/persistence/schema/hrm-schema.sql",
  ]
}

env "dev" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = local.hrm_src

  migration {
    dir    = "file://migrations/hrm"
    format = "goose"
  }
}

env "test" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = local.hrm_src

  migration {
    dir    = "file://migrations/hrm"
    format = "goose"
  }
}

env "ci" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = local.hrm_src

  migration {
    dir    = "file://migrations/hrm"
    format = "goose"
  }
}
