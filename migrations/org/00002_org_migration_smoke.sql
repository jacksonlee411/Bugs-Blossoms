-- +goose Up
CREATE TABLE __org_migration_smoke (
    id integer PRIMARY KEY
);

-- +goose Down
DROP TABLE __org_migration_smoke;
