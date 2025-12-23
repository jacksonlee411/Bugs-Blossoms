variable "db_url" {
  type    = string
  default = getenv("DB_URL")
}

variable "atlas_dev_url" {
  type    = string
  default = getenv("ATLAS_DEV_URL")
}

locals {
  person_src = [
    "file://modules/person/infrastructure/atlas/core_deps.sql",
    "file://modules/person/infrastructure/persistence/schema/person-schema.sql",
  ]
  org_src = [
    "file://modules/org/infrastructure/atlas/core_deps.sql",
    "file://modules/org/infrastructure/persistence/schema/org-schema.sql",
  ]
}

env "dev" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = local.person_src

  migration {
    dir    = "file://migrations/person"
    format = "goose"
  }
}

env "test" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = local.person_src

  migration {
    dir    = "file://migrations/person"
    format = "goose"
  }
}

env "ci" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = local.person_src

  migration {
    dir    = "file://migrations/person"
    format = "goose"
  }
}

env "org_dev" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = local.org_src

  migration {
    dir    = "file://migrations/org"
    format = "goose"
  }
}

env "org_ci" {
  url = var.db_url
  dev = var.atlas_dev_url
  src = local.org_src

  migration {
    dir    = "file://migrations/org"
    format = "goose"
  }
}
