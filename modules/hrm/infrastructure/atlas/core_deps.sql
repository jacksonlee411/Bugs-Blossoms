CREATE TABLE tenants (
  id uuid PRIMARY KEY
);

CREATE TABLE uploads (
  id serial PRIMARY KEY,
  tenant_id uuid REFERENCES tenants (id) ON DELETE CASCADE
);

CREATE TABLE currencies (
  code varchar(3) NOT NULL PRIMARY KEY
);

