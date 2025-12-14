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
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: employee_contacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.employee_contacts (
    id integer NOT NULL,
    employee_id integer NOT NULL,
    type character varying(255) NOT NULL,
    value character varying(255) NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: employee_contacts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.employee_contacts_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: employee_contacts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.employee_contacts_id_seq OWNED BY public.employee_contacts.id;


--
-- Name: employee_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.employee_meta (
    employee_id integer NOT NULL,
    primary_language character varying(255),
    secondary_language character varying(255),
    tin character varying(255),
    pin character varying(255),
    notes text,
    birth_date date,
    hire_date date,
    resignation_date date
);


--
-- Name: employee_positions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.employee_positions (
    employee_id integer NOT NULL,
    position_id integer NOT NULL
);


--
-- Name: employees; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.employees (
    id integer NOT NULL,
    tenant_id uuid NOT NULL,
    first_name character varying(255) NOT NULL,
    last_name character varying(255) NOT NULL,
    middle_name character varying(255),
    email character varying(255) NOT NULL,
    phone character varying(255),
    salary numeric(9,2) NOT NULL,
    salary_currency_id character varying(3),
    hourly_rate numeric(9,2) NOT NULL,
    coefficient double precision NOT NULL,
    avatar_id integer,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: employees_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.employees_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: employees_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.employees_id_seq OWNED BY public.employees.id;


--
-- Name: positions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.positions (
    id integer NOT NULL,
    tenant_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: positions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.positions_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: positions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.positions_id_seq OWNED BY public.positions.id;


--
-- Name: employee_contacts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employee_contacts ALTER COLUMN id SET DEFAULT nextval('public.employee_contacts_id_seq'::regclass);


--
-- Name: employees id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employees ALTER COLUMN id SET DEFAULT nextval('public.employees_id_seq'::regclass);


--
-- Name: positions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.positions ALTER COLUMN id SET DEFAULT nextval('public.positions_id_seq'::regclass);


--
-- Name: employee_contacts employee_contacts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employee_contacts
    ADD CONSTRAINT employee_contacts_pkey PRIMARY KEY (id);


--
-- Name: employee_meta employee_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employee_meta
    ADD CONSTRAINT employee_meta_pkey PRIMARY KEY (employee_id);


--
-- Name: employee_positions employee_positions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employee_positions
    ADD CONSTRAINT employee_positions_pkey PRIMARY KEY (employee_id, position_id);


--
-- Name: employees employees_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT employees_pkey PRIMARY KEY (id);


--
-- Name: employees employees_tenant_id_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT employees_tenant_id_email_key UNIQUE (tenant_id, email);


--
-- Name: employees employees_tenant_id_phone_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT employees_tenant_id_phone_key UNIQUE (tenant_id, phone);


--
-- Name: positions positions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.positions
    ADD CONSTRAINT positions_pkey PRIMARY KEY (id);


--
-- Name: positions positions_tenant_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.positions
    ADD CONSTRAINT positions_tenant_id_name_key UNIQUE (tenant_id, name);


--
-- Name: employees_email_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX employees_email_idx ON public.employees USING btree (email);


--
-- Name: employees_first_name_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX employees_first_name_idx ON public.employees USING btree (first_name);


--
-- Name: employees_last_name_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX employees_last_name_idx ON public.employees USING btree (last_name);


--
-- Name: employees_phone_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX employees_phone_idx ON public.employees USING btree (phone);


--
-- Name: employees_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX employees_tenant_id_idx ON public.employees USING btree (tenant_id);


--
-- Name: positions_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX positions_tenant_id_idx ON public.positions USING btree (tenant_id);


--
-- Name: employee_contacts employee_contacts_employee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employee_contacts
    ADD CONSTRAINT employee_contacts_employee_id_fkey FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;


--
-- Name: employee_meta employee_meta_employee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employee_meta
    ADD CONSTRAINT employee_meta_employee_id_fkey FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;


--
-- Name: employee_positions employee_positions_employee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employee_positions
    ADD CONSTRAINT employee_positions_employee_id_fkey FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;


--
-- Name: employee_positions employee_positions_position_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employee_positions
    ADD CONSTRAINT employee_positions_position_id_fkey FOREIGN KEY (position_id) REFERENCES public.positions(id) ON DELETE CASCADE;


--
-- Name: employees employees_avatar_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT employees_avatar_id_fkey FOREIGN KEY (avatar_id) REFERENCES public.uploads(id) ON DELETE SET NULL;


--
-- Name: employees employees_salary_currency_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT employees_salary_currency_id_fkey FOREIGN KEY (salary_currency_id) REFERENCES public.currencies(code) ON DELETE SET NULL;


--
-- Name: employees employees_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT employees_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;


--
-- Name: positions positions_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.positions
    ADD CONSTRAINT positions_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--


