-- Final setup: Grant table ownership to the application user (primus_lens)
-- This runs after all migrations to ensure the application user can operate all tables

DO
$$
    DECLARE
        r RECORD;
        app_user TEXT := 'primus-lens';
    BEGIN
        RAISE NOTICE 'Setting table ownership to user: %', app_user;
        FOR r IN SELECT tablename FROM pg_tables WHERE schemaname = 'public'
            LOOP
                EXECUTE format('ALTER TABLE public.%I OWNER TO %I;', r.tablename, app_user);
            END LOOP;
    END
$$;

-- Also grant privileges on sequences
DO
$$
    DECLARE
        r RECORD;
        app_user TEXT := 'primus-lens';
    BEGIN
        FOR r IN SELECT sequencename FROM pg_sequences WHERE schemaname = 'public'
            LOOP
                EXECUTE format('ALTER SEQUENCE public.%I OWNER TO %I;', r.sequencename, app_user);
            END LOOP;
    END
$$;

-- Grant schema privileges
GRANT USAGE ON SCHEMA public TO "primus-lens";
GRANT CREATE ON SCHEMA public TO "primus-lens";
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO "primus-lens";
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO "primus-lens";
