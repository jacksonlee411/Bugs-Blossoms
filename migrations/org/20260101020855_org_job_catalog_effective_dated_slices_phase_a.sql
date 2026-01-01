-- +goose Up
-- create "org_job_family_group_slices" table
CREATE TABLE "public"."org_job_family_group_slices" ("tenant_id" uuid NOT NULL, "id" uuid NOT NULL DEFAULT gen_random_uuid(), "job_family_group_id" uuid NOT NULL, "name" text NOT NULL, "is_active" boolean NOT NULL DEFAULT true, "effective_date" date NOT NULL, "end_date" date NOT NULL DEFAULT '9999-12-31', "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("id"), CONSTRAINT "org_job_family_group_slices_no_overlap" EXCLUDE USING gist ("tenant_id" WITH =, "job_family_group_id" WITH =, (daterange(effective_date, (end_date + 1), '[)'::text)) WITH &&), CONSTRAINT "org_job_family_group_slices_tenant_id_id_key" UNIQUE ("tenant_id", "id"), CONSTRAINT "org_job_family_group_slices_group_fk" FOREIGN KEY ("tenant_id", "job_family_group_id") REFERENCES "public"."org_job_family_groups" ("tenant_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT, CONSTRAINT "org_job_family_group_slices_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "org_job_family_group_slices_effective_check" CHECK (effective_date <= end_date));
-- create index "org_job_family_group_slices_tenant_group_effective_idx" to table: "org_job_family_group_slices"
CREATE INDEX "org_job_family_group_slices_tenant_group_effective_idx" ON "public"."org_job_family_group_slices" ("tenant_id", "job_family_group_id", "effective_date");
-- create "org_job_family_slices" table
CREATE TABLE "public"."org_job_family_slices" ("tenant_id" uuid NOT NULL, "id" uuid NOT NULL DEFAULT gen_random_uuid(), "job_family_id" uuid NOT NULL, "name" text NOT NULL, "is_active" boolean NOT NULL DEFAULT true, "effective_date" date NOT NULL, "end_date" date NOT NULL DEFAULT '9999-12-31', "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("id"), CONSTRAINT "org_job_family_slices_no_overlap" EXCLUDE USING gist ("tenant_id" WITH =, "job_family_id" WITH =, (daterange(effective_date, (end_date + 1), '[)'::text)) WITH &&), CONSTRAINT "org_job_family_slices_tenant_id_id_key" UNIQUE ("tenant_id", "id"), CONSTRAINT "org_job_family_slices_family_fk" FOREIGN KEY ("tenant_id", "job_family_id") REFERENCES "public"."org_job_families" ("tenant_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT, CONSTRAINT "org_job_family_slices_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "org_job_family_slices_effective_check" CHECK (effective_date <= end_date));
-- create index "org_job_family_slices_tenant_family_effective_idx" to table: "org_job_family_slices"
CREATE INDEX "org_job_family_slices_tenant_family_effective_idx" ON "public"."org_job_family_slices" ("tenant_id", "job_family_id", "effective_date");
-- create "org_job_level_slices" table
CREATE TABLE "public"."org_job_level_slices" ("tenant_id" uuid NOT NULL, "id" uuid NOT NULL DEFAULT gen_random_uuid(), "job_level_id" uuid NOT NULL, "name" text NOT NULL, "display_order" integer NOT NULL DEFAULT 0, "is_active" boolean NOT NULL DEFAULT true, "effective_date" date NOT NULL, "end_date" date NOT NULL DEFAULT '9999-12-31', "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("id"), CONSTRAINT "org_job_level_slices_no_overlap" EXCLUDE USING gist ("tenant_id" WITH =, "job_level_id" WITH =, (daterange(effective_date, (end_date + 1), '[)'::text)) WITH &&), CONSTRAINT "org_job_level_slices_tenant_id_id_key" UNIQUE ("tenant_id", "id"), CONSTRAINT "org_job_level_slices_level_fk" FOREIGN KEY ("tenant_id", "job_level_id") REFERENCES "public"."org_job_levels" ("tenant_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT, CONSTRAINT "org_job_level_slices_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "org_job_level_slices_display_order_check" CHECK (display_order >= 0), CONSTRAINT "org_job_level_slices_effective_check" CHECK (effective_date <= end_date));
-- create index "org_job_level_slices_tenant_level_effective_idx" to table: "org_job_level_slices"
CREATE INDEX "org_job_level_slices_tenant_level_effective_idx" ON "public"."org_job_level_slices" ("tenant_id", "job_level_id", "effective_date");
-- create "org_job_profile_slices" table
CREATE TABLE "public"."org_job_profile_slices" ("tenant_id" uuid NOT NULL, "id" uuid NOT NULL DEFAULT gen_random_uuid(), "job_profile_id" uuid NOT NULL, "name" text NOT NULL, "description" text NULL, "is_active" boolean NOT NULL DEFAULT true, "external_refs" jsonb NOT NULL DEFAULT '{}', "effective_date" date NOT NULL, "end_date" date NOT NULL DEFAULT '9999-12-31', "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("id"), CONSTRAINT "org_job_profile_slices_no_overlap" EXCLUDE USING gist ("tenant_id" WITH =, "job_profile_id" WITH =, (daterange(effective_date, (end_date + 1), '[)'::text)) WITH &&), CONSTRAINT "org_job_profile_slices_tenant_id_id_key" UNIQUE ("tenant_id", "id"), CONSTRAINT "org_job_profile_slices_profile_fk" FOREIGN KEY ("tenant_id", "job_profile_id") REFERENCES "public"."org_job_profiles" ("tenant_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT, CONSTRAINT "org_job_profile_slices_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "org_job_profile_slices_effective_check" CHECK (effective_date <= end_date), CONSTRAINT "org_job_profile_slices_external_refs_is_object_check" CHECK (jsonb_typeof(external_refs) = 'object'::text));
-- create index "org_job_profile_slices_tenant_profile_effective_idx" to table: "org_job_profile_slices"
CREATE INDEX "org_job_profile_slices_tenant_profile_effective_idx" ON "public"."org_job_profile_slices" ("tenant_id", "job_profile_id", "effective_date");
-- create "org_job_profile_slice_job_families" table
CREATE TABLE "public"."org_job_profile_slice_job_families" ("tenant_id" uuid NOT NULL, "job_profile_slice_id" uuid NOT NULL, "job_family_id" uuid NOT NULL, "is_primary" boolean NOT NULL DEFAULT false, "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now(), PRIMARY KEY ("tenant_id", "job_profile_slice_id", "job_family_id"), CONSTRAINT "org_job_profile_slice_job_families_family_fk" FOREIGN KEY ("tenant_id", "job_family_id") REFERENCES "public"."org_job_families" ("tenant_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT, CONSTRAINT "org_job_profile_slice_job_families_slice_fk" FOREIGN KEY ("tenant_id", "job_profile_slice_id") REFERENCES "public"."org_job_profile_slices" ("tenant_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "org_job_profile_slice_job_families_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "org_job_profile_slice_job_families_primary_unique" to table: "org_job_profile_slice_job_families"
CREATE UNIQUE INDEX "org_job_profile_slice_job_families_primary_unique" ON "public"."org_job_profile_slice_job_families" ("tenant_id", "job_profile_slice_id") WHERE (is_primary = true);
-- create index "org_job_profile_slice_job_families_tenant_family_slice_idx" to table: "org_job_profile_slice_job_families"
CREATE INDEX "org_job_profile_slice_job_families_tenant_family_slice_idx" ON "public"."org_job_profile_slice_job_families" ("tenant_id", "job_family_id", "job_profile_slice_id");
-- drop "__org_migration_smoke" table
DROP TABLE "public"."__org_migration_smoke";

-- +goose Down
-- reverse: drop "__org_migration_smoke" table
CREATE TABLE "public"."__org_migration_smoke" ("id" integer NOT NULL, PRIMARY KEY ("id"));
-- reverse: create index "org_job_profile_slice_job_families_tenant_family_slice_idx" to table: "org_job_profile_slice_job_families"
DROP INDEX "public"."org_job_profile_slice_job_families_tenant_family_slice_idx";
-- reverse: create index "org_job_profile_slice_job_families_primary_unique" to table: "org_job_profile_slice_job_families"
DROP INDEX "public"."org_job_profile_slice_job_families_primary_unique";
-- reverse: create "org_job_profile_slice_job_families" table
DROP TABLE "public"."org_job_profile_slice_job_families";
-- reverse: create index "org_job_profile_slices_tenant_profile_effective_idx" to table: "org_job_profile_slices"
DROP INDEX "public"."org_job_profile_slices_tenant_profile_effective_idx";
-- reverse: create "org_job_profile_slices" table
DROP TABLE "public"."org_job_profile_slices";
-- reverse: create index "org_job_level_slices_tenant_level_effective_idx" to table: "org_job_level_slices"
DROP INDEX "public"."org_job_level_slices_tenant_level_effective_idx";
-- reverse: create "org_job_level_slices" table
DROP TABLE "public"."org_job_level_slices";
-- reverse: create index "org_job_family_slices_tenant_family_effective_idx" to table: "org_job_family_slices"
DROP INDEX "public"."org_job_family_slices_tenant_family_effective_idx";
-- reverse: create "org_job_family_slices" table
DROP TABLE "public"."org_job_family_slices";
-- reverse: create index "org_job_family_group_slices_tenant_group_effective_idx" to table: "org_job_family_group_slices"
DROP INDEX "public"."org_job_family_group_slices_tenant_group_effective_idx";
-- reverse: create "org_job_family_group_slices" table
DROP TABLE "public"."org_job_family_group_slices";
