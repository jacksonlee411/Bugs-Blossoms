-- +migrate Up

-- DEV-PLAN-019D: tenant domains + per-tenant auth settings + SSO connections + superadmin audit logs

-- Change CREATE_TABLE: tenant_domains
CREATE TABLE tenant_domains (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    hostname varchar(255) NOT NULL,
    is_primary bool NOT NULL DEFAULT false,
    verification_token varchar(64) NOT NULL,
    last_verification_attempt_at timestamptz NULL,
    last_verification_error text NULL,
    verified_at timestamptz NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX tenant_domains_hostname_uq ON tenant_domains(hostname);
CREATE UNIQUE INDEX tenant_domains_tenant_primary_uq ON tenant_domains(tenant_id) WHERE is_primary;
CREATE INDEX tenant_domains_tenant_id_idx ON tenant_domains(tenant_id);

ALTER TABLE tenant_domains
    ADD CONSTRAINT tenant_domains_hostname_lower_check CHECK (hostname = lower(hostname));
ALTER TABLE tenant_domains
    ADD CONSTRAINT tenant_domains_hostname_no_port_check CHECK (position(':' in hostname) = 0);
ALTER TABLE tenant_domains
    ADD CONSTRAINT tenant_domains_hostname_no_scheme_check CHECK (position('://' in hostname) = 0);
ALTER TABLE tenant_domains
    ADD CONSTRAINT tenant_domains_hostname_no_path_check CHECK (position('/' in hostname) = 0);

-- Backfill from legacy tenants.domain (mark as verified to avoid breaking existing routing)
	INSERT INTO tenant_domains (tenant_id, hostname, is_primary, verification_token, verified_at, created_at, updated_at)
	SELECT
	    t.id,
	    lower(trim(t.domain)),
	    true,
	    replace(gen_random_uuid()::text, '-', '') || replace(gen_random_uuid()::text, '-', ''),
	    now(),
	    now(),
	    now()
	FROM tenants t
	WHERE t.domain IS NOT NULL AND length(trim(t.domain)) > 0;

-- Change CREATE_TABLE: tenant_auth_settings
CREATE TABLE tenant_auth_settings (
    tenant_id uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    identity_mode varchar(20) NOT NULL DEFAULT 'legacy',
    allow_password bool NOT NULL DEFAULT true,
    allow_google bool NOT NULL DEFAULT true,
    allow_sso bool NOT NULL DEFAULT false,
    updated_at timestamptz DEFAULT now() NOT NULL
);

ALTER TABLE tenant_auth_settings
    ADD CONSTRAINT tenant_auth_settings_identity_mode_check CHECK (identity_mode IN ('legacy', 'kratos'));

-- Backfill defaults for existing tenants
INSERT INTO tenant_auth_settings (tenant_id, identity_mode, allow_password, allow_google, allow_sso, updated_at)
SELECT t.id, 'legacy', true, true, false, now()
FROM tenants t
ON CONFLICT (tenant_id) DO NOTHING;

-- Change CREATE_TABLE: tenant_sso_connections
CREATE TABLE tenant_sso_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    connection_id varchar(64) NOT NULL,
    display_name varchar(255) NOT NULL,
    protocol varchar(10) NOT NULL,
    enabled bool NOT NULL DEFAULT false,
    jackson_base_url varchar(255) NOT NULL,
    kratos_provider_id varchar(64) NOT NULL,
    saml_metadata_url varchar(1024) NULL,
    saml_metadata_xml text NULL,
    oidc_issuer varchar(1024) NULL,
    oidc_client_id varchar(255) NULL,
    oidc_client_secret_ref varchar(255) NULL,
    last_test_status varchar(20) NULL,
    last_test_error text NULL,
    last_test_at timestamptz NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX tenant_sso_connections_tenant_connection_uq ON tenant_sso_connections(tenant_id, connection_id);
CREATE INDEX tenant_sso_connections_tenant_id_idx ON tenant_sso_connections(tenant_id);
CREATE INDEX tenant_sso_connections_tenant_enabled_idx ON tenant_sso_connections(tenant_id, enabled);

ALTER TABLE tenant_sso_connections
    ADD CONSTRAINT tenant_sso_connections_protocol_check CHECK (protocol IN ('saml', 'oidc'));
ALTER TABLE tenant_sso_connections
    ADD CONSTRAINT tenant_sso_connections_saml_config_check CHECK (
        protocol != 'saml' OR (saml_metadata_url IS NOT NULL OR saml_metadata_xml IS NOT NULL)
    );
ALTER TABLE tenant_sso_connections
    ADD CONSTRAINT tenant_sso_connections_oidc_config_check CHECK (
        protocol != 'oidc' OR (oidc_issuer IS NOT NULL AND oidc_client_id IS NOT NULL AND oidc_client_secret_ref IS NOT NULL)
    );
ALTER TABLE tenant_sso_connections
    ADD CONSTRAINT tenant_sso_connections_oidc_secret_ref_check CHECK (
        protocol != 'oidc' OR (oidc_client_secret_ref LIKE 'ENV:%' OR oidc_client_secret_ref LIKE 'FILE:%')
    );

-- Change CREATE_TABLE: superadmin_audit_logs
CREATE TABLE superadmin_audit_logs (
    id bigserial PRIMARY KEY,
    actor_user_id int8 NULL,
    actor_email_snapshot varchar(255) NOT NULL,
    actor_name_snapshot varchar(255) NULL,
    tenant_id uuid NULL REFERENCES tenants(id) ON DELETE SET NULL,
    action varchar(64) NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    ip_address inet NULL,
    user_agent text NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);

CREATE INDEX superadmin_audit_logs_tenant_time_idx ON superadmin_audit_logs(tenant_id, created_at DESC);
CREATE INDEX superadmin_audit_logs_time_idx ON superadmin_audit_logs(created_at DESC);

-- +migrate Down

DROP TABLE IF EXISTS superadmin_audit_logs;
DROP TABLE IF EXISTS tenant_sso_connections;
DROP TABLE IF EXISTS tenant_auth_settings;
DROP TABLE IF EXISTS tenant_domains;
