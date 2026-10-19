CREATE INDEX idx_tasks_project_status ON tasks(project_id, status);
CREATE INDEX idx_tasks_project_deadline ON tasks(project_id, deadline);
CREATE INDEX idx_dependencies_predecessor ON dependencies(depends_on_id);