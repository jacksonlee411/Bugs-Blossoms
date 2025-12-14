-- +goose Up
CREATE TABLE __hrm_migration_smoke (
  id integer PRIMARY KEY
);

-- +goose Down
DROP TABLE __hrm_migration_smoke;

