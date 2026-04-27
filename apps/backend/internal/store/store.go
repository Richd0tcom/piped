package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite"
	"github.com/richd0tcom/piped/internal/models"
)



type Store struct {
	db *sql.DB
}

func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single writer
	s := &Store{db: db}
	return s, s.migrate()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS deployments (
			id                   VARCHAR(255) PRIMARY KEY,
			name                 VARCHAR(255) NOT NULL,
			source_type          VARCHAR(64) NOT NULL,
			resource_url              TEXT,
			git_branch           VARCHAR(255),
			git_commit           TEXT,
			env_vars             TEXT,
			status               VARCAHR(255) NOT NULL,
			image_tag            VARCHAR(255),
			active_container_id  VARCHAR(63),
			standby_container_id VARCHAR(63),
			caddy_route          VARCHAR(255),
			port                 INTEGER,
			created_at           DATETIME NOT NULL,
			updated_at           DATETIME NOT NULL
		);
		CREATE TABLE IF NOT EXISTS log_lines (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			deployment_id VARCHAR(255) NOT NULL,
			stream        TEXT NOT NULL,
			text          TEXT NOT NULL,
			sequence      INTEGER NOT NULL,
			created_at    DATETIME NOT NULL,
			FOREIGN KEY (deployment_id) REFERENCES deployments(id)
		);
		CREATE INDEX IF NOT EXISTS idx_logs_deployment ON log_lines(deployment_id, sequence);
	`)
	return err
}

func (s *Store) CreateDeployment(ctx context.Context, d *models.Deployment) error {
	env, _ := json.Marshal(d.EnvVars)
	_, err := s.db.ExecContext(ctx,`
		INSERT INTO deployments (id,name,source_type,resource_url,git_branch,git_commit,env_vars,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.Name, d.SourceType, d.ResourceURL, d.GitBranch, d.GitCommit, string(env),
		d.Status, d.CreatedAt, d.UpdatedAt,
	)
	return err
}

func (s *Store) UpdateDeployment(ctx context.Context, d *models.Deployment) error {
	env, _ := json.Marshal(d.EnvVars)
	d.UpdatedAt = time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE deployments SET
			status=?, image_tag=?, active_container_id=?, standby_container_id=?,
			caddy_route=?, port=?, env_vars=?, updated_at=?
		WHERE id=?`,
		d.Status, d.ImageTag, d.ActiveContainerID, d.StandbyContainerID,
		d.CaddyRoute, d.Port, string(env), d.UpdatedAt, d.ID,
	)
	return err
}

func (s *Store) GetDeployment(ctx context.Context, id string) (*models.Deployment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT * FROM deployments WHERE id=?`, id)
	return scanDeployment(row)
}

func (s *Store) ListDeployments(ctx context.Context) ([]*models.Deployment, error) {
	rows, err := s.db.QueryContext(ctx,`SELECT * FROM deployments ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.Deployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Store) InsertLogLine(l *models.LogLine) error {
	_, err := s.db.Exec(`
		INSERT INTO log_lines (deployment_id,stream,text,sequence,created_at)
		VALUES (?,?,?,?,?)`,
		l.DeploymentID, l.Stream, l.Text, l.Sequence, l.CreatedAt,
	)
	return err
}

func (s *Store) GetLogs(ctx context.Context, deploymentID string, fromSequence int64) ([]*models.LogLine, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,deployment_id,stream,text,sequence,created_at
		FROM log_lines WHERE deployment_id=? AND sequence>=?
		ORDER BY sequence ASC`, deploymentID, fromSequence)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.LogLine
	for rows.Next() {
		l := &models.LogLine{}
		if err := rows.Scan(&l.ID, &l.DeploymentID, &l.Stream, &l.Text, &l.Sequence, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDeployment(row scanner) (*models.Deployment, error) {
	d := &models.Deployment{}
	var envJSON string
	err := row.Scan(
		&d.ID, &d.Name, &d.SourceType, &d.ResourceURL, &d.GitBranch, &d.GitCommit, &envJSON,
		&d.Status, &d.ImageTag, &d.ActiveContainerID, &d.StandbyContainerID,
		&d.CaddyRoute, &d.Port, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(envJSON), &d.EnvVars)
	return d, nil
}