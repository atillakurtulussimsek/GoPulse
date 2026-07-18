-- 0002_sessions.sql — Oturum (session) tablosu
-- DB tabanlı oturum yönetimi: rastgele token'lar sunucu tarafında saklanır,
-- cookie yalnızca token'ı taşır. Süresi dolan/iptal edilen oturumlar silinir.

CREATE TABLE sessions (
    id         TEXT     PRIMARY KEY,                 -- rastgele oturum token'ı
    user_id    INTEGER  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);

CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
