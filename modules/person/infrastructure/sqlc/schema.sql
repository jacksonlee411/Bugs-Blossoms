--
-- PostgreSQL database dump
--
-- Dumped from database version 17.7 (Debian 17.7-3.pgdg13+1)
-- Dumped by pg_dump version 17.7 (Debian 17.7-3.pgdg13+1)
SET statement_timeout = 0;

SET lock_timeout = 0;

SET idle_in_transaction_session_timeout = 0;

SET transaction_timeout = 0;

SET client_encoding = 'UTF8';

SET standard_conforming_strings = ON;

SELECT
    pg_catalog.set_config('search_path', '', FALSE);

SET check_function_bodies = FALSE;

SET xmloption = content;

SET client_min_messages = warning;

SET row_security = OFF;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: persons; Type: TABLE; Schema: public; Owner: -
--
CREATE TABLE public.persons (
    tenant_id uuid NOT NULL,
    person_uuid uuid DEFAULT gen_random_uuid () NOT NULL,
    pernr text NOT NULL,
    display_name text NOT NULL,
    status text DEFAULT 'active' ::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT persons_pernr_trim_check CHECK (((pernr = btrim(pernr)) AND (pernr <> ''::text))),
    CONSTRAINT persons_status_check CHECK ((status = ANY (ARRAY['active'::text, 'inactive'::text])))
);

--
-- Name: persons persons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--
ALTER TABLE ONLY public.persons
    ADD CONSTRAINT persons_pkey PRIMARY KEY (person_uuid);

--
-- Name: persons persons_tenant_id_pernr_key; Type: CONSTRAINT; Schema: public; Owner: -
--
ALTER TABLE ONLY public.persons
    ADD CONSTRAINT persons_tenant_id_pernr_key UNIQUE (tenant_id, pernr);

--
-- Name: persons persons_tenant_id_person_uuid_key; Type: CONSTRAINT; Schema: public; Owner: -
--
ALTER TABLE ONLY public.persons
    ADD CONSTRAINT persons_tenant_id_person_uuid_key UNIQUE (tenant_id, person_uuid);

--
-- Name: persons_tenant_display_name_idx; Type: INDEX; Schema: public; Owner: -
--
CREATE INDEX persons_tenant_display_name_idx ON public.persons USING btree (tenant_id, display_name);

--
-- Name: persons_tenant_pernr_idx; Type: INDEX; Schema: public; Owner: -
--
CREATE INDEX persons_tenant_pernr_idx ON public.persons USING btree (tenant_id, pernr);

--
-- Name: persons persons_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--
ALTER TABLE ONLY public.persons
    ADD CONSTRAINT persons_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants (id) ON DELETE CASCADE;

--
-- PostgreSQL database dump complete
--
