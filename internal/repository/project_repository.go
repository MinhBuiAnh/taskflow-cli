package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/MinhBuiAnh/taskflow-cli/internal/domain"
)

var ErrNotFound = errors.New("record not found")

type ProjectRepository struct{ db *sql.DB }

func NewProjectRepository(db *sql.DB) *ProjectRepository { return &ProjectRepository{db: db} }

func (r *ProjectRepository) Create(ctx context.Context, p *domain.Project) error {
	if p.Status == "" {
		p.Status = domain.ProjectActive
	}
	if err := p.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	result, err := r.db.ExecContext(ctx, `INSERT INTO projects(name, description, status, created_at, updated_at, archived_at) VALUES (?, ?, ?, ?, ?, ?)`, p.Name, p.Description, p.Status, stamp(p.CreatedAt), stamp(p.UpdatedAt), nullableStamp(p.ArchivedAt))
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	p.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get project ID: %w", err)
	}
	return nil
}
func (r *ProjectRepository) Get(ctx context.Context, id int64) (*domain.Project, error) {
	p := new(domain.Project)
	var created, updated string
	var archived sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, name, description, status, created_at, updated_at, archived_at FROM projects WHERE id = ?`, id).Scan(&p.ID, &p.Name, &p.Description, &p.Status, &created, &updated, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	p.CreatedAt, err = parseStamp(created)
	if err != nil {
		return nil, err
	}
	p.UpdatedAt, err = parseStamp(updated)
	if err != nil {
		return nil, err
	}
	if archived.Valid {
		t, e := parseStamp(archived.String)
		if e != nil {
			return nil, e
		}
		p.ArchivedAt = &t
	}
	return p, nil
}
func (r *ProjectRepository) List(ctx context.Context) ([]domain.Project, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, description, status, created_at, updated_at, archived_at FROM projects ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	out := make([]domain.Project, 0)
	for rows.Next() {
		var p domain.Project
		var created, updated string
		var archived sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Status, &created, &updated, &archived); err != nil {
			return nil, err
		}
		p.CreatedAt, err = parseStamp(created)
		if err != nil {
			return nil, err
		}
		p.UpdatedAt, err = parseStamp(updated)
		if err != nil {
			return nil, err
		}
		if archived.Valid {
			t, e := parseStamp(archived.String)
			if e != nil {
				return nil, e
			}
			p.ArchivedAt = &t
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
func (r *ProjectRepository) Update(ctx context.Context, p *domain.Project) error {
	if err := p.Validate(); err != nil {
		return err
	}
	p.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `UPDATE projects SET name=?, description=?, status=?, updated_at=?, archived_at=? WHERE id=?`, p.Name, p.Description, p.Status, stamp(p.UpdatedAt), nullableStamp(p.ArchivedAt), p.ID)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *ProjectRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM projects WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}