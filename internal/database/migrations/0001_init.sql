-- 0001_init.sql — GoPulse başlangıç şeması
-- Bu dosya migration mekanizması tarafından tek seferlik (idempotent)
-- uygulanır; schema_migrations tablosunda versiyonu kaydedilir.

-- Panele giriş yapabilen yetkili kullanıcılar (Multi-User, Single-Tenant).
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- İzlenen hedefler.
CREATE TABLE monitors (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    name             TEXT    NOT NULL,
    type             TEXT    NOT NULL,              -- 'http' | 'tcp' | ...
    target           TEXT    NOT NULL,              -- URL veya host:port
    interval_seconds INTEGER NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,    -- 0/1 (boolean)
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Kontrol geçmişi. Pruning mekanizması bu tabloyu budar.
CREATE TABLE check_results (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    status     TEXT    NOT NULL,                    -- 'up' | 'down' | 'pending'
    latency_ms INTEGER NOT NULL DEFAULT 0,
    message    TEXT    NOT NULL DEFAULT '',
    checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Pruning ve monitor bazlı sorgular için indeksler.
CREATE INDEX idx_check_results_checked_at ON check_results(checked_at);
CREATE INDEX idx_check_results_monitor_id ON check_results(monitor_id);

-- Bildirim kanalları / alıcıları (panelden dinamik yönetilir).
CREATE TABLE notification_channels (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    type    TEXT    NOT NULL,                       -- 'telegram' | 'smtp' | ...
    label   TEXT    NOT NULL,
    config  TEXT    NOT NULL DEFAULT '{}',          -- kanala özel ayarlar (JSON)
    enabled INTEGER NOT NULL DEFAULT 1
);

-- Monitor ↔ bildirim kanalı eşlemesi (M:N).
CREATE TABLE monitor_channels (
    monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    PRIMARY KEY (monitor_id, channel_id)
);
