-- +goose Up
CREATE TABLE __person_migration_smoke (
  id integer PRIMARY KEY
);

-- +goose Down
DROP TABLE __person_migration_smoke;

