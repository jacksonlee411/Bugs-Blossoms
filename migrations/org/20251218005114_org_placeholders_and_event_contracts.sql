-- +goose Up
-- create "org_attribute_inheritance_rules" table
CREATE TABLE "public"."org_attribute_inheritance_rules" ("tenant_id" uuid NOT NULL, "id" uuid NOT NULL DEFAULT gen_random_uuid(), "hierarchy_type" text NOT NULL, "attribute_name" text NOT NULL, "can_override" boolean NOT NULL DEFAULT false, "inheritance_break_node_type" text NULL, "effective_date" timestamptz NOT NULL, "end_date" timestamptz NOT NULL DEFAULT '9999-12-31 00:00:00+00', "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("id"), CONSTRAINT "org_attribute_inheritance_rules_no_overlap" EXCLUDE USING gist ("tenant_id" WITH =, "hierarchy_type" WITH =, "attribute_name" WITH =, (tstzrange(effective_date, end_date, '[)'::text)) WITH &&), CONSTRAINT "org_attribute_inheritance_rules_tenant_id_id_key" UNIQUE ("tenant_id", "id"), CONSTRAINT "org_attribute_inheritance_rules_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "org_attribute_inheritance_rules_effective_check" CHECK (effective_date < end_date));
-- create index "org_attribute_inheritance_rules_tenant_hierarchy_attribute_effe" to table: "org_attribute_inheritance_rules"
CREATE INDEX "org_attribute_inheritance_rules_tenant_hierarchy_attribute_effe" ON "public"."org_attribute_inheritance_rules" ("tenant_id", "hierarchy_type", "attribute_name", "effective_date");
-- create "org_change_requests" table
CREATE TABLE "public"."org_change_requests" ("tenant_id" uuid NOT NULL, "id" uuid NOT NULL DEFAULT gen_random_uuid(), "request_id" text NOT NULL, "requester_id" uuid NOT NULL, "status" text NOT NULL DEFAULT 'draft', "payload_schema_version" integer NOT NULL DEFAULT 1, "payload" jsonb NOT NULL, "notes" text NULL, "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("id"), CONSTRAINT "org_change_requests_tenant_id_id_key" UNIQUE ("tenant_id", "id"), CONSTRAINT "org_change_requests_tenant_id_request_id_key" UNIQUE ("tenant_id", "request_id"), CONSTRAINT "org_change_requests_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "org_change_requests_status_check" CHECK (status = ANY (ARRAY['draft'::text, 'submitted'::text, 'approved'::text, 'rejected'::text, 'cancelled'::text])));
-- create index "org_change_requests_tenant_requester_status_updated_idx" to table: "org_change_requests"
CREATE INDEX "org_change_requests_tenant_requester_status_updated_idx" ON "public"."org_change_requests" ("tenant_id", "requester_id", "status", "updated_at" DESC);
-- create "org_roles" table
CREATE TABLE "public"."org_roles" ("tenant_id" uuid NOT NULL, "id" uuid NOT NULL DEFAULT gen_random_uuid(), "code" character varying(64) NOT NULL, "name" character varying(255) NOT NULL, "description" text NULL, "is_system" boolean NOT NULL DEFAULT true, "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("id"), CONSTRAINT "org_roles_tenant_id_code_key" UNIQUE ("tenant_id", "code"), CONSTRAINT "org_roles_tenant_id_id_key" UNIQUE ("tenant_id", "id"), CONSTRAINT "org_roles_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "org_roles_tenant_name_idx" to table: "org_roles"
CREATE INDEX "org_roles_tenant_name_idx" ON "public"."org_roles" ("tenant_id", "name");
-- create "org_role_assignments" table
CREATE TABLE "public"."org_role_assignments" ("tenant_id" uuid NOT NULL, "id" uuid NOT NULL DEFAULT gen_random_uuid(), "role_id" uuid NOT NULL, "subject_type" text NOT NULL DEFAULT 'user', "subject_id" uuid NOT NULL, "org_node_id" uuid NOT NULL, "effective_date" timestamptz NOT NULL, "end_date" timestamptz NOT NULL DEFAULT '9999-12-31 00:00:00+00', "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("id"), CONSTRAINT "org_role_assignments_no_overlap" EXCLUDE USING gist ("tenant_id" WITH =, "role_id" WITH =, "subject_type" WITH =, "subject_id" WITH =, "org_node_id" WITH =, (tstzrange(effective_date, end_date, '[)'::text)) WITH &&), CONSTRAINT "org_role_assignments_tenant_id_id_key" UNIQUE ("tenant_id", "id"), CONSTRAINT "org_role_assignments_org_node_fk" FOREIGN KEY ("tenant_id", "org_node_id") REFERENCES "public"."org_nodes" ("tenant_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT, CONSTRAINT "org_role_assignments_role_fk" FOREIGN KEY ("tenant_id", "role_id") REFERENCES "public"."org_roles" ("tenant_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT, CONSTRAINT "org_role_assignments_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "org_role_assignments_effective_check" CHECK (effective_date < end_date), CONSTRAINT "org_role_assignments_subject_type_check" CHECK (subject_type = ANY (ARRAY['user'::text, 'group'::text])));
-- create index "org_role_assignments_tenant_node_effective_idx" to table: "org_role_assignments"
CREATE INDEX "org_role_assignments_tenant_node_effective_idx" ON "public"."org_role_assignments" ("tenant_id", "org_node_id", "effective_date");
-- create index "org_role_assignments_tenant_subject_effective_idx" to table: "org_role_assignments"
CREATE INDEX "org_role_assignments_tenant_subject_effective_idx" ON "public"."org_role_assignments" ("tenant_id", "subject_type", "subject_id", "effective_date");
-- drop "__org_migration_smoke" table
DROP TABLE "public"."__org_migration_smoke";

-- +goose Down
-- reverse: drop "__org_migration_smoke" table
CREATE TABLE "public"."__org_migration_smoke" ("id" integer NOT NULL, PRIMARY KEY ("id"));
-- reverse: create index "org_role_assignments_tenant_subject_effective_idx" to table: "org_role_assignments"
DROP INDEX "public"."org_role_assignments_tenant_subject_effective_idx";
-- reverse: create index "org_role_assignments_tenant_node_effective_idx" to table: "org_role_assignments"
DROP INDEX "public"."org_role_assignments_tenant_node_effective_idx";
-- reverse: create "org_role_assignments" table
DROP TABLE "public"."org_role_assignments";
-- reverse: create index "org_roles_tenant_name_idx" to table: "org_roles"
DROP INDEX "public"."org_roles_tenant_name_idx";
-- reverse: create "org_roles" table
DROP TABLE "public"."org_roles";
-- reverse: create index "org_change_requests_tenant_requester_status_updated_idx" to table: "org_change_requests"
DROP INDEX "public"."org_change_requests_tenant_requester_status_updated_idx";
-- reverse: create "org_change_requests" table
DROP TABLE "public"."org_change_requests";
-- reverse: create index "org_attribute_inheritance_rules_tenant_hierarchy_attribute_effe" to table: "org_attribute_inheritance_rules"
DROP INDEX "public"."org_attribute_inheritance_rules_tenant_hierarchy_attribute_effe";
-- reverse: create "org_attribute_inheritance_rules" table
DROP TABLE "public"."org_attribute_inheritance_rules";
