CREATE TABLE dependencies (
    task_id INTEGER NOT NULL,
    depends_on_id INTEGER NOT NULL,
    project_id INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (task_id, depends_on_id),
    CHECK (task_id <> depends_on_id),
    FOREIGN KEY (task_id, project_id) REFERENCES tasks(id, project_id) ON DELETE RESTRICT,
    FOREIGN KEY (depends_on_id, project_id) REFERENCES tasks(id, project_id) ON DELETE RESTRICT
);