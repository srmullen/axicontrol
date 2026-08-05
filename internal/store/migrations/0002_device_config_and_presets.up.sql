CREATE TABLE device_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    model INTEGER NOT NULL DEFAULT 1,
    penlift INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO device_config (id) VALUES (1);

CREATE TABLE presets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    speed_pendown INTEGER NOT NULL DEFAULT 25,
    speed_penup INTEGER NOT NULL DEFAULT 75,
    accel INTEGER NOT NULL DEFAULT 75,
    pen_pos_down INTEGER NOT NULL DEFAULT 40,
    pen_pos_up INTEGER NOT NULL DEFAULT 60,
    pen_rate_lower INTEGER NOT NULL DEFAULT 50,
    pen_rate_raise INTEGER NOT NULL DEFAULT 50,
    pen_delay_down INTEGER NOT NULL DEFAULT 0,
    pen_delay_up INTEGER NOT NULL DEFAULT 0,
    const_speed INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
