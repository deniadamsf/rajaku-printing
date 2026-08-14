-- Rollback modul auth
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS menu_permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
-- extension pgcrypto dibiarkan (mungkin dipakai migration lain)
